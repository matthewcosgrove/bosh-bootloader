package acceptance_test

import (
	"time"

	acceptance "github.com/cloudfoundry/bosh-bootloader/acceptance-tests"
	"github.com/cloudfoundry/bosh-bootloader/acceptance-tests/actors"
	"github.com/cloudfoundry/bosh-bootloader/testhelpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("lbs test", func() {
	var (
		bbl   actors.BBL
		gcp   actors.GCP
		state acceptance.State
	)

	BeforeEach(func() {
		configuration, err := acceptance.LoadConfig()
		Expect(err).NotTo(HaveOccurred())

		state = acceptance.NewState(configuration.StateFileDir)
		bbl = actors.NewBBL(configuration.StateFileDir, pathToBBL, configuration, "lbs-env")
		gcp = actors.NewGCP(configuration)

		session := bbl.Up("--name", bbl.PredefinedEnvID(), "--no-director")
		Eventually(session, 40*time.Minute).Should(gexec.Exit(0))
	})

	AfterEach(func() {
		session := bbl.Destroy()
		Eventually(session, 10*time.Minute).Should(gexec.Exit())
	})

	It("successfully creates, updates, and deletes cf lbs", func() {
		var urlToSSLCert string

		By("creating a load balancer", func() {
			certPath, err := testhelpers.WriteContentsToTempFile(testhelpers.BBL_CERT)
			Expect(err).NotTo(HaveOccurred())

			keyPath, err := testhelpers.WriteContentsToTempFile(testhelpers.BBL_KEY)
			Expect(err).NotTo(HaveOccurred())

			session := bbl.CreateLB("cf", certPath, keyPath, "")
			Eventually(session, 10*time.Minute).Should(gexec.Exit(0))
		})

		By("confirming that target pools exist", func() {
			targetPools := []string{bbl.PredefinedEnvID() + "-cf-ssh-proxy", bbl.PredefinedEnvID() + "-cf-tcp-router"}
			for _, p := range targetPools {
				targetPool, err := gcp.GetTargetPool(p)
				Expect(err).NotTo(HaveOccurred())
				Expect(targetPool.Name).NotTo(BeNil())
				Expect(targetPool.Name).To(Equal(p))
			}

			targetHTTPSProxy, err := gcp.GetTargetHTTPSProxy(bbl.PredefinedEnvID() + "-https-proxy")
			Expect(err).NotTo(HaveOccurred())

			Expect(targetHTTPSProxy.SslCertificates).To(HaveLen(1))
			urlToSSLCert = targetHTTPSProxy.SslCertificates[0]
		})

		By("verifying that the bbl lbs output contains the cf lbs", func() {
			session := bbl.LBs()
			Eventually(session).Should(gexec.Exit(0))
			stdout := string(session.Out.Contents())
			Expect(stdout).To(MatchRegexp("CF Router LB: .*"))
			Expect(stdout).To(MatchRegexp("CF SSH Proxy LB: .*"))
			Expect(stdout).To(MatchRegexp("CF TCP Router LB: .*"))
			Expect(stdout).To(MatchRegexp("CF WebSocket LB: .*"))
		})

		By("updating the load balancer", func() {
			otherCertPath, err := testhelpers.WriteContentsToTempFile(testhelpers.OTHER_BBL_CERT)
			Expect(err).NotTo(HaveOccurred())

			otherKeyPath, err := testhelpers.WriteContentsToTempFile(testhelpers.OTHER_BBL_KEY)
			Expect(err).NotTo(HaveOccurred())

			session := bbl.UpdateLB(otherCertPath, otherKeyPath, "")
			Eventually(session, 10*time.Minute).Should(gexec.Exit(0))
		})

		By("confirming that the cert gets updated", func() {
			targetHTTPSProxy, err := gcp.GetTargetHTTPSProxy(bbl.PredefinedEnvID() + "-https-proxy")
			Expect(err).NotTo(HaveOccurred())

			Expect(targetHTTPSProxy.SslCertificates).To(HaveLen(1))
			Expect(targetHTTPSProxy.SslCertificates[0]).NotTo(BeEmpty())
			Expect(targetHTTPSProxy.SslCertificates[0]).NotTo(Equal(urlToSSLCert))
		})

		By("deleting lbs", func() {
			session := bbl.DeleteLBs()
			Eventually(session, 15*time.Minute).Should(gexec.Exit(0))
		})

		By("confirming that the target pools do not exist", func() {
			targetPools := []string{bbl.PredefinedEnvID() + "-cf-ssh-proxy", bbl.PredefinedEnvID() + "-cf-tcp-router"}
			for _, p := range targetPools {
				_, err := gcp.GetTargetPool(p)
				Expect(err).To(MatchError(MatchRegexp(`The resource 'projects\/.+` + p + `' was not found`)))
			}
		})
	})
})
