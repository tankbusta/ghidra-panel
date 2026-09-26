GRADLE?=./gradlew
SQLITE?=sqlite3

.PHONY: build
build:
	$(GRADLE) jar

../test.db:
	$(MAKE) -C .. test.db

.PHONY: integration
integration: ../test.db
	SAMPLE_USER=richard SAMPLE_PASS=richard $(GRADLE) integration

.PHONY: clean
clean:
	#$(GRADLE) clean
	rm -rf target
