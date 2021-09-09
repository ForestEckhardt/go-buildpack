package integration_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudfoundry/libbuildpack/cutlass"
	"github.com/onsi/gomega/format"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	. "github.com/onsi/gomega"
)

var settings struct {
	Buildpack struct {
		Version string
		Path    string
	}
	Dynatrace struct {
		App *cutlass.App
		URI string
	}
	FixturesPath string
	GitHubToken  string
	Platform     string
}

func init() {
	flag.BoolVar(&cutlass.Cached, "cached", true, "cached buildpack")
	flag.StringVar(&cutlass.DefaultMemory, "memory", "128M", "default memory for pushed apps")
	flag.StringVar(&cutlass.DefaultDisk, "disk", "384M", "default disk for pushed apps")
	flag.StringVar(&settings.Buildpack.Version, "version", "", "version to use (builds if empty)")
	flag.StringVar(&settings.GitHubToken, "github-token", "", "use the token to make GitHub API requests")
	flag.StringVar(&settings.Platform, "platform", "cf", "platform to run against")
}

func TestIntegration(t *testing.T) {
	format.MaxLength = 0

	var (
		Expect     = NewWithT(t).Expect
		Eventually = NewWithT(t).Eventually

		packagedBuildpack cutlass.VersionedBuildpackPackage
	)

	root, err := cutlass.FindRoot()
	Expect(err).NotTo(HaveOccurred())

	settings.FixturesPath = filepath.Join(root, "fixtures")

	if settings.Buildpack.Version == "" {
		packagedBuildpack, err = cutlass.PackageUniquelyVersionedBuildpack(os.Getenv("CF_STACK"), true)
		Expect(err).NotTo(HaveOccurred())

		settings.Buildpack.Path = packagedBuildpack.File

		info, err := os.Stat(settings.Buildpack.Path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Size() < 1024*1024*1024).To(BeTrue(), "Buildpack file size must be less than 1G")

		settings.Buildpack.Version = packagedBuildpack.Version
	}

	err = cutlass.CreateOrUpdateBuildpack("override", filepath.Join(settings.FixturesPath, "util", "override_buildpack"), "")
	Expect(err).NotTo(HaveOccurred())

	Expect(cutlass.CopyCfHome()).To(Succeed())
	cutlass.SeedRandom()

	settings.Dynatrace.App = cutlass.New(filepath.Join(settings.FixturesPath, "util", "dynatrace"))
	settings.Dynatrace.App.SetEnv("BP_DEBUG", "true")
	settings.Dynatrace.App.Buildpacks = []string{"go_buildpack"}

	Expect(settings.Dynatrace.App.Push()).To(Succeed())
	Eventually(func() ([]string, error) {
		return settings.Dynatrace.App.InstanceStates()
	}, 60*time.Second).Should(Equal([]string{"RUNNING"}))

	settings.Dynatrace.URI, err = settings.Dynatrace.App.GetUrl("")
	Expect(err).NotTo(HaveOccurred())

	suite := spec.New("integration", spec.Report(report.Terminal{}), spec.Parallel())
	suite("Default", testDefault)
	suite("Dep", testDep)
	suite("Dynatrace", testDynatrace)
	suite("Errors", testErrors)
	suite("Glide", testGlide)
	suite("GoToolchain", testGoToolchain)
	suite("Godep", testGodep)
	suite("Modules", testModules)
	suite("MultiBuildpack", testMultiBuildpack)
	suite("Override", testOverride)

	if cutlass.Cached {
		suite("Offline", testOffline)
	} else {
		suite("Cache", testCache)
		suite("Proxy", testProxy)
	}

	suite.Run(t)

	DestroyApp(t, settings.Dynatrace.App)
	Expect(cutlass.RemovePackagedBuildpack(packagedBuildpack)).To(Succeed())
	Expect(cutlass.DeleteBuildpack("override")).To(Succeed())
	Expect(cutlass.DeleteOrphanedRoutes()).To(Succeed())
}

func PushAppAndConfirm(t *testing.T, app *cutlass.App) {
	var (
		Expect     = NewWithT(t).Expect
		Eventually = NewWithT(t).Eventually
	)

	Expect(app.Push()).To(Succeed())
	Eventually(func() ([]string, error) { return app.InstanceStates() }, 20*time.Second).Should(Equal([]string{"RUNNING"}))
	Expect(app.ConfirmBuildpack(settings.Buildpack.Version)).To(Succeed())
}

func DestroyApp(t *testing.T, app *cutlass.App) *cutlass.App {
	var Expect = NewWithT(t).Expect
	Expect(app.Destroy()).To(Succeed())

	return nil
}
