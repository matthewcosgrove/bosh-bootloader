#/bin/bash

command -v yaml-patch >/dev/null 2>&1 || { echo >&2 "Requires yaml-patch but not installed. Aborting. 'go get github.com/krishicks/yaml-patch/cmd/yaml-patch'"; exit 1; }
cat concourse.yml | yaml-patch -o operations/update-atc-properties.yml > /tmp/patched-concourse.yml
