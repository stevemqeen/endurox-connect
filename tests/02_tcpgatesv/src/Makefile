# Do recursive builds
all:
	$(MAKE) -C ubftab
	$(MAKE) -C testsv
	$(MAKE) -C testcl

clean:
	$(MAKE) -C ubftab clean
	$(MAKE) -C testsv clean
	$(MAKE) -C testcl clean

.PHONY: clean all
