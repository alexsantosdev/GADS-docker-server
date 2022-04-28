package ios

import (
	"bytes"
	"errors"
	"os"
	"os/exec"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/imagemounter"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/testmanagerd"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	log "github.com/sirupsen/logrus"
)

var udid = os.Getenv("DEVICE_UDID")
var bundleid = os.Getenv("WDA_BUNDLEID")
var testrunnerbundleid = bundleid
var xctestconfig = "WebDriverAgentRunner.xctest"
var wda_port = os.Getenv("WDA_PORT")
var wda_mjpeg_port = os.Getenv("MJPEG_PORT")
var appium_port = "4723"
var device_os_version = os.Getenv("DEVICE_OS_VERSION")
var device_name = os.Getenv("DEVICE_NAME")

func startAppiumIOS() {

	capabilities := `{"mjpegServerPort": ` + wda_mjpeg_port +
		`, "clearSystemFiles": "false",` +
		`"webDriverAgentUrl":"'http:$deviceIP:` + wda_port + `'",` +
		`"preventWDAAttachments": "true",` +
		`"simpleIsVisibleCheck": "false",` +
		`"wdaLocalPort": "'` + wda_port + `'",` +
		`"platformVersion": "'` + device_os_version + `'",` +
		`"automationName":"XCUITest",` +
		`"platformName": "iOS",` +
		`"deviceName": "'` + device_name + `'",` +
		`"wdaLaunchTimeout": "120000",` +
		`"wdaConnectionTimeout": "240000",` +
		`"settings[mjpegServerScreenshotQuality]": 25,` +
		`"settings[mjpegScalingFactor]": 50,` +
		`"settings[mjpegServerFramerate]": 20}`

	commandString := "appium -p " + appium_port + " --udid" + udid + " --log-timestamp --default-capabilities '" + capabilities + "'"
	cmd := exec.Command("bash", "-c", commandString)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.WithFields(log.Fields{
			"event": "start_appium_ios",
		}).Error("test")
		return
	}
	log.WithFields(log.Fields{
		"event": "start_appium_ios",
	}).Info("test")
}

func startWDA() {

	device, err := ios.GetDevice(udid)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "run_wda",
		}).Error("Could not get device when installing app. Error: " + err.Error())
	}

	go func() {
		err := testmanagerd.RunXCUIWithBundleIds(bundleid,
			testrunnerbundleid,
			xctestconfig,
			device,
			[]string{},
			[]string{"USE_PORT=" + wda_port, "MJPEG_SERVER_PORT=" + wda_mjpeg_port})

		log.WithFields(log.Fields{
			"event": "run_wda",
		}).Error("Failed running wda. Error: " + err.Error())
	}()
}

func stopWDA() {
	err := testmanagerd.CloseXCUITestRunner()
	if err != nil {
		log.WithFields(log.Fields{
			"event": "stop_wda",
		}).Error("Failed closing wda runner. Error: " + err.Error())
	}
}

func installWDA() error {
	err := installApp("WebDriverAgent.ipa")
	return err
}

func installApp(fileName string) error {
	filePath := "/opt/" + fileName

	device, err := ios.GetDevice(udid)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "install_app",
		}).Error("Could not get device when installing app. Error: " + err.Error())
		return errors.New("Failed installing application")
	}

	conn, err := zipconduit.New(device)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "install_app",
		}).Error("Could not create zipconduit when installing app. Error: " + err.Error())
		return errors.New("Failed installing application")
	}

	err = conn.SendFile(filePath)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "install_app",
		}).Error("Could not install app. Error: " + err.Error())
		return errors.New("Failed installing application")
	}

	log.WithFields(log.Fields{
		"event": "install_app",
	}).Info("Could not install app. Error: " + err.Error())
	return nil
}

func mountDiskImages() error {
	device, err := ios.GetDevice(udid)

	if err != nil {
		log.WithFields(log.Fields{
			"event": "mount_dev_images",
		}).Error("Could not get device when mounting dev images. Error: " + err.Error())
		return errors.New("Failed mounting disk images")
	}

	mountConn, err := imagemounter.New(device)
	signatures, err := mountConn.ListImages()

	if len(signatures) == 0 {
		basedir := "/opt/devimages"

		err = imagemounter.FixDevImage(device, basedir)
		log.WithFields(log.Fields{
			"event": "mount_dev_images",
		}).Error("Could not get device when mounting dev images. Error: " + err.Error())
		return errors.New("Failed mounting disk images")
	} else {
		log.WithFields(log.Fields{
			"event": "mount_dev_images",
		}).Info("DevImages are mounted on device with UDID: '" + udid)
		return nil
	}
}

func uninstallAppInternal(bundle_id string) error {
	device, err := ios.GetDevice(udid)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "uninstall_ios_app",
		}).Error("Could not get device when uninstalling app with bundleID:'" + bundle_id + "'. Error: " + err.Error())
		return errors.New("Error")
	}

	svc, err := installationproxy.New(device)
	if err != nil {
		log.WithFields(log.Fields{
			"event": "uninstall_ios_app",
		}).Error("Failed connecting installationproxy when uninstalling app with bundleID:'" + bundle_id + "'. Error: " + err.Error())
		return errors.New("Error")
	}

	err = svc.Uninstall(bundle_id)

	if err != nil {
		log.WithFields(log.Fields{
			"event": "uninstall_ios_app",
		}).Error("Failed uninstalling app with bundleID:'" + bundle_id + "'. Error: " + err.Error())
		return errors.New("Error")
	}
	return nil
}

func getInstalledApps() {

}
