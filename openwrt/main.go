package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"dagger/openwrt/internal/dagger"
)

type Openwrt struct {
	Version string
	Target  string
}

func New(
	// OpenWRT version
	// +default="24.10.3"
	version string,
	// OpenWRT build target
	// +default="bcm27xx/bcm2711"
	target string,
) *Openwrt {
	return &Openwrt{
		Version: version,
		Target:  target,
	}
}

func (m *Openwrt) Profiles(ctx context.Context) (string, error) {
	return m.ImageBuilder().WithExec([]string{"make", "info"}).Stdout(ctx)
}

func (m *Openwrt) Build(
	ctx context.Context,
	// OpenWRT target profile
	// +optional
	profile string,
	// List of additional included (or excluded) packages
	// +optional
	packages []string,
	// List of disabled services
	// +optional
	disabledServices []string,
	// RootFS partition size (default is 100MB)
	// +optional
	rootSizeMB string,
) *dagger.Directory {
	cmd := []string{"make", "image"}

	if profile != "" {
		cmd = append(cmd, fmt.Sprintf("PROFILE=%s", profile))
	}
	if len(packages) > 0 {
		cmd = append(cmd, fmt.Sprintf("PACKAGES=%s", strings.Join(packages, " ")))
	}
	if len(disabledServices) > 0 {
		cmd = append(cmd, fmt.Sprintf("DISABLED_SERVICES=%s", strings.Join(disabledServices, " ")))
	}
	if rootSizeMB != "" {
		cmd = append(cmd, fmt.Sprintf("ROOTFS_PARTSIZE=%s", rootSizeMB))
	}

	return m.ImageBuilder().
		WithExec(cmd).
		Directory(fmt.Sprintf("bin/targets/%s", m.Target))
}

func (m *Openwrt) Manifest(
	ctx context.Context,
	// OpenWRT target profile
	// +optional
	profile string,
	// List of additional included (or excluded) packages
	// +optional
	packages []string,
) (string, error) {
	cmd := []string{"make", "manifest"}

	if profile != "" {
		cmd = append(cmd, fmt.Sprintf("PROFILE=%s", profile))
	}
	if len(packages) > 0 {
		cmd = append(cmd, fmt.Sprintf("PACKAGES=%s", strings.Join(packages, " ")))
	}

	return m.ImageBuilder().WithExec(cmd).Stdout(ctx)
}

func (m *Openwrt) ImageBuilder() *dagger.Container {
	base := dag.
		Container().
		From("debian:trixie").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{
			"apt-get", "install", "-y",
			"build-essential",
			"file",
			"gawk",
			"gettext",
			"git",
			"libncurses-dev",
			"libssl-dev",
			"python3",
			"python3-setuptools", // replaces python3-distutils?
			"rsync",
			"unzip",
			"wget",
			"xsltproc",
			"zlib1g-dev",
			"zstd",
		})

	tarball := imageBuilderTarball(m.Version, m.Target)
	imageBuilder := base.
		WithWorkdir("/src").
		WithMountedFile("/tmp/openwrt-imagebuilder.tar.zst", tarball).
		WithExec([]string{
			"tar",
			"--zstd",
			"--strip-components=1",
			"--extract",
		}, dagger.ContainerWithExecOpts{RedirectStdin: "/tmp/openwrt-imagebuilder.tar.zst"}).
		Directory("")

	return base.
		WithWorkdir("/src").
		WithMountedDirectory("", imageBuilder).
		WithMountedCache("dl", dag.CacheVolume("openwrt-downloaded-packages"))
}

func imageBuilderTarball(version string, target string) *dagger.File {
	var downloadPath string
	var downloadSuffix string
	if version == "" {
		downloadPath = "snapshots"
		downloadSuffix = ""
	} else {
		downloadPath = "releases/" + version
		downloadSuffix = "-" + version
	}

	// Examples
	//   Stable:   https://downloads.openwrt.org/releases/24.10.3/targets/bcm27xx/bcm2711/openwrt-imagebuilder-24.10.3-bcm27xx-bcm2711.Linux-x86_64.tar.zst
	//   Snapshot: https://downloads.openwrt.org/snapshots/targets/bcm27xx/bcm2711/openwrt-imagebuilder-bcm27xx-bcm2711.Linux-x86_64.tar.zst
	return dag.HTTP(fmt.Sprintf(
		"https://downloads.openwrt.org/%s/targets/%s/openwrt-imagebuilder%s-%s.Linux-x86_64.tar.zst",
		downloadPath,
		target,
		downloadSuffix,
		strings.ReplaceAll(target, "/", "-"),
	))
}

func (m *Openwrt) Diff(
	ctx context.Context,
	newPackages *dagger.File,
	oldPackages *dagger.File,
) (string, error) {
	newStr, err := newPackages.Contents(ctx)
	if err != nil {
		return "", err
	}
	oldStr, err := oldPackages.Contents(ctx)
	if err != nil {
		return "", err
	}

	oldMap := make(map[string]string)
	for _, line := range strings.Split(oldStr, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) == 2 {
			oldMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	newMap := make(map[string]string)
	for _, line := range strings.Split(newStr, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			newMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	pkgNames := make(map[string]struct{})
	for name := range newMap {
		pkgNames[name] = struct{}{}
	}
	for name := range oldMap {
		pkgNames[name] = struct{}{}
	}

	sortedNames := make([]string, 0, len(pkgNames))
	for name := range pkgNames {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	var diffs []string
	for _, name := range sortedNames {
		newVersion, inNew := newMap[name]
		oldVersion, inOld := oldMap[name]

		if inNew && inOld {
			if newVersion != oldVersion {
				diffs = append(diffs, fmt.Sprintf("%s: %s -> %s", name, oldVersion, newVersion))
			}
		} else if inNew {
			diffs = append(diffs, fmt.Sprintf("+ %s: %s", name, newVersion))
		} else if inOld {
			diffs = append(diffs, fmt.Sprintf("- %s: %s", name, oldVersion))
		}
	}

	return strings.Join(diffs, "\n"), nil
}
