package web

import (
	"go.mkw.re/ghidra-panel/ghidra"
	"log"
	"net/http"
)

func (s *Server) handleDeleteRepo(wr http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(wr, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ident, ok := s.checkAuth(req)
	if !ok {
		http.Error(wr, "Not authorized", http.StatusUnauthorized)
		return
	}

	if err := req.ParseForm(); err != nil {
		http.Error(wr, "Bad request", http.StatusBadRequest)
		return
	}
	repo := req.PostForm.Get("repo")
	confirm := req.PostForm.Get("confirm")

	// Check for missing form data.
	if repo == "" || confirm != repo {
		http.Redirect(wr, req, redirectUrl(req, map[string]string{"status": "missing_fields"}), http.StatusSeeOther)
		return
	}

	// Allow super admins to delete any repository
	if !s.isSuperAdmin(ident) {
		// Fetch user state from the database and Ghidra
		result, err := s.fetchUserPermission(req, ident, repo)
		if err != nil {
			log.Println("Failed to get repository user:", err)
			http.Redirect(wr, req, redirectUrl(req, map[string]string{"status": "internal_error"}), http.StatusSeeOther)
			return
		}

		// Verify the user has admin access to the repository
		if result.Permission != ghidra.Permission_ADMIN {
			http.Error(wr, "Forbidden", http.StatusForbidden)
			return
		}

		// Verify the repository has only zero or one user
		reply, err := s.Client.GetRepository(req.Context(), &ghidra.GetRepositoryRequest{
			Repository: repo,
		})
		if err != nil {
			log.Println("Failed to get repository:", err)
			http.Redirect(wr, req, redirectUrl(req, map[string]string{"status": "internal_error"}), http.StatusSeeOther)
			return
		}
		if reply.Repository == nil {
			http.NotFound(wr, req)
			return
		}
		if len(reply.Users) > 1 {
			http.Error(wr, "Cannot delete repository with multiple users", http.StatusForbidden)
			return
		}
	}

	// Delete the repository
	if _, err := s.Client.DeleteRepository(req.Context(), &ghidra.DeleteRepositoryRequest{
		Repository: repo,
	}); err != nil {
		log.Println("Failed to delete repository:", err)
		http.Redirect(wr, req, redirectUrl(req, map[string]string{"status": "internal_error"}), http.StatusSeeOther)
		return
	}

	http.Redirect(wr, req, "/?status=delete_repo_success", http.StatusSeeOther)
}
