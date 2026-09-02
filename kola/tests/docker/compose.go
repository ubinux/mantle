// Copyright The Mantle Authors.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"

	"github.com/coreos/go-semver/semver"
	"github.com/flatcar/mantle/kola/cluster"
	"github.com/flatcar/mantle/kola/register"
)

const dockerComposeVersion = "2.40.3"

func init() {
	register.Register(&register.Test{
		Name:        "docker.compose-sysext",
		Run:         dockerComposeSysext,
		ClusterSize: 1,
		Distros:     []string{"cl"},
		// This test is normally not related to the cloud environment.
		Platforms:  []string{"qemu", "qemu-unpriv", "azure"},
		MinVersion: semver.Version{Major: 3185},
	})
}

func dockerComposeSysext(c cluster.TestCluster) {
	m := c.Machines()[0]
	projectName := "kola-compose-test"
	composeFile := "/home/core/compose.yaml"

	c.Run("download-and-merge", func(c cluster.TestCluster) {
		cmd := fmt.Sprintf(`
set -euo pipefail

arch=$(uname -m)
case "${arch}" in
	x86_64|amd64)
		sysext_arch=x86-64
		;;
	aarch64|arm64)
		sysext_arch=arm64
		;;
	*)
		echo "unsupported architecture: ${arch}"
		exit 1
		;;
esac

version=%[1]q
url="https://extensions.flatcar.org/extensions/docker-compose-${version}-${sysext_arch}.raw"

sudo curl --fail --location --retry 5 --retry-delay 2 --output /etc/extensions/docker-compose.raw "${url}"
sudo systemctl restart systemd-sysext
sudo systemctl daemon-reload

sudo systemd-sysext status --json=pretty | jq -e '.[] | select(.hierarchy == "/usr") | .extensions[] | select(. == "docker-compose")' >/dev/null
`, dockerComposeVersion)
		_ = c.MustSSH(m, cmd)
	})

	c.Run("version-check", func(c cluster.TestCluster) {
		cmd := fmt.Sprintf(`
set -euo pipefail

got=$(docker compose version --short)
if [ "${got}" != %[1]q ]; then
	echo "unexpected docker compose version: got ${got}, expected %[1]s"
	exit 1
fi
`, dockerComposeVersion)
		_ = c.MustSSH(m, cmd)
	})

	c.Run("compose-up", func(c cluster.TestCluster) {
		cmd := fmt.Sprintf(`
set -euo pipefail

sudo systemctl start docker

cat >%[1]q <<'EOF'
services:
  app:
    image: ghcr.io/flatcar/busybox
    command: ["sleep", "infinity"]
EOF

docker compose --project-name %[2]q -f %[1]q down || true
docker compose --project-name %[2]q -f %[1]q up -d
docker compose --project-name %[2]q -f %[1]q ps --services --filter status=running | grep -x app >/dev/null
`, composeFile, projectName)
		_ = c.MustSSH(m, cmd)
	})

	c.Run("compose-down-cleanup", func(c cluster.TestCluster) {
		cmd := fmt.Sprintf(`
set -euo pipefail

docker compose --project-name %[1]q -f %[2]q down

if [ -n "$(docker ps -a --filter label=com.docker.compose.project=%[1]q --quiet)" ]; then
	echo "containers for %[1]s still exist after docker compose down"
	exit 1
fi

if [ -n "$(docker network ls --filter label=com.docker.compose.project=%[1]q --quiet)" ]; then
	echo "networks for %[1]s still exist after docker compose down"
	exit 1
fi
`, projectName, composeFile)
		_ = c.MustSSH(m, cmd)
	})
}
