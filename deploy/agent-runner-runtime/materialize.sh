#!/bin/sh
set -eu

# Materialize vendored Agent Runner tools from /opt/vendor into /opt/*.
# Each component dir is either an unpacked tree (with bin/) or a single tar.

vendor_root="${1:-/opt/vendor}"

flatten_extract() {
	src="$1"
	dest="$2"
	tmp="$(mktemp -d)"
	case "${src}" in
	*.tar.xz) tar -xJf "${src}" -C "${tmp}" ;;
	*.tar.gz|*.tgz) tar -xzf "${src}" -C "${tmp}" ;;
	*.tar|*.tar.tar) tar -xf "${src}" -C "${tmp}" ;;
	*)
		echo "unsupported archive: ${src}" >&2
		exit 1
		;;
	esac
	set -- "${tmp}"/*
	if [ "$#" -eq 1 ] && [ -d "$1" ]; then
		cp -a "$1"/. "${dest}/"
	else
		cp -a "${tmp}"/. "${dest}/"
	fi
	rm -rf "${tmp}"
}

materialize() {
	name="$1"
	dest="$2"
	required="$3"
	src="${vendor_root}/${name}"
	if [ ! -d "${src}" ]; then
		if [ "${required}" = "1" ]; then
			echo "missing ${src}; put the ${name} package under deploy/agent-runner-runtime/${name}/" >&2
			exit 1
		fi
		echo "optional component ${name} not provided, skipping"
		return 0
	fi
	mkdir -p "${dest}"
	if [ -d "${src}/bin" ]; then
		cp -a "${src}/." "${dest}/"
		return 0
	fi
	archive="$(find "${src}" -maxdepth 1 -type f \( \
		-name '*.tar.xz' -o -name '*.tar.gz' -o -name '*.tgz' -o -name '*.tar.tar' -o -name '*.tar' \
		\) | head -n 1)"
	if [ -n "${archive}" ]; then
		flatten_extract "${archive}" "${dest}"
		return 0
	fi
	if [ "${required}" = "1" ]; then
		echo "${src} has neither bin/ nor a tar archive" >&2
		exit 1
	fi
	echo "optional component ${name} is empty, skipping"
}

materialize codex-package /opt/codex 1
materialize node /opt/node 1
materialize jdk /opt/jdk 1
materialize python /opt/python 1
materialize maven /opt/maven 1
materialize claude /opt/claude 0

if [ ! -x /opt/python/bin/python3 ] && [ -x /opt/python/bin/python3.12 ]; then
	ln -s python3.12 /opt/python/bin/python3
fi
if [ -x /opt/python/bin/python3 ] && [ ! -e /opt/python/bin/python ]; then
	ln -s python3 /opt/python/bin/python
fi
if [ ! -x /opt/python/bin/pip3 ] && [ ! -x /opt/python/bin/pip ] && [ -x /opt/python/bin/python3 ]; then
	/opt/python/bin/python3 -m ensurepip --upgrade >/dev/null 2>&1 || true
fi

chmod -R a+rX /opt/codex /opt/node /opt/jdk /opt/python /opt/maven
if [ -d /opt/claude/bin ]; then
	chmod -R a+rX /opt/claude
fi
for bindir in /opt/codex/bin /opt/node/bin /opt/jdk/bin /opt/python/bin /opt/maven/bin /opt/claude/bin; do
	if [ -d "${bindir}" ]; then
		find "${bindir}" -maxdepth 1 -type f -exec chmod a+x {} \;
	fi
done
