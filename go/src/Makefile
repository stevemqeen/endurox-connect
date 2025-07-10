# exutil is Go library. ubftab must be built first.

all:
	$(MAKE) -C ubftab
	$(MAKE) -C restincl
	$(MAKE) -C restoutsv
	$(MAKE) -C tcpgatesv

clean:
	$(MAKE) -C ubftab clean
	$(MAKE) -C restincl clean
	$(MAKE) -C restoutsv clean
	$(MAKE) -C tcpgatesv clean
