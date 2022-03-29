SHELL		= /bin/bash
DESTDIR		?=
package		= gostatuses
program		= statuses
busname		= com.github.canalguada.gostatuses
git_branch	= master
version		= 0.0.1
revision	= 1
release_dir	= bin
prefix		= ~/.local
bindir		= $(prefix)/bin
datadir		= $(prefix)/share/

.DEFAULT_GOAL := default

.PHONY: build
build:
	/usr/bin/go build -o $(release_dir)/$(program) -ldflags="-s" ./cmd/$(program)

.PHONY: build-widgets
build-widgets:
	/usr/bin/go build -o $(release_dir)/widgets -ldflags="-s" ./cmd/widgets

.PHONY: default
default: build

.PHONY: all
all: build build-widgets

.PHONY: clean
clean:
	: # do nothing

.PHONY: distclean
distclean:
	$(SUDO) find $(release_dir)/ -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} \;

# .PHONY: deb
# deb:
#         version=$(version) \
#         revision=$(revision) \
#         release_dir=$(release_dir) \
#         ./package.sh

.PHONY: dist
dist:
	mkdir -p $(release_dir)
	git archive --format=tar.gz \
		-o $(release_dir)/$(package)-$(version).tar.gz \
		--prefix=$(package)-$(version)/ \
		$(git_branch)

.PHONY: install-bin
install-bin:
	install -d $(DESTDIR)$(bindir)
	install -m755 $(release_dir)/$(program) $(DESTDIR)$(bindir)/$(program)

.PHONY: install-widgets
install-widgets:
	install -d $(DESTDIR)$(bindir)
	install -m755 $(release_dir)/widgets $(DESTDIR)$(bindir)/widgets

.PHONY: install
install: install-bin install-widgets
	install -d $(DESTDIR)$(datadir)/systemd/user
	install -m644 service/dbus-$(busname).service \
		$(DESTDIR)$(datadir)/systemd/user/
	install -d $(DESTDIR)$(datadir)/dbus-1/services
	install -m644 service/$(busname).service \
		$(DESTDIR)$(datadir)/dbus-1/services/
	systemctl --user daemon-reload

.PHONY: uninstall-bin
uninstall-bin:
	rm -f $(DESTDIR)$(bindir)/$(program)

.PHONY: uninstall-widgets
uninstall-widgets:
	rm -f $(DESTDIR)$(bindir)/widgets

.PHONY: uninstall
uninstall: uninstall-bin uninstall-widgets
	rm -f $(DESTDIR)$(datadir)/systemd/user/dbus-$(busname).service
	rmdir --ignore-fail-on-non-empty $(DESTDIR)$(datadir)/systemd/user
	rm -f $(DESTDIR)$(datadir)/dbus-1/services/$(busname).service
	rmdir --ignore-fail-on-non-empty $(DESTDIR)$(datadir)/dbus-1/services
	systemctl --user daemon-reload

