package ios_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/imagemounter"
	"github.com/danielpaulus/go-ios/ios/testmanagerd"
	"github.com/shamanec/GADS-docker-server/config"
	log "github.com/sirupsen/logrus"
)

func SetupDevice() {
	go startUsbmuxd()

	config.GetDevice()

	// Pair the supervised device
	err := retry.Do(
		func() error {
			err := pairDevice()
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(2*time.Second),
	)
	if err != nil {
		panic(err)
	}

	// Mount developer disk images
	err = retry.Do(
		func() error {
			err := mountDeveloperImage()
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(2*time.Second),
	)
	if err != nil {
		panic(err)
	}

	// Install WebDriverAgent and start it
	err = retry.Do(
		func() error {
			err := prepareWDA()
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(2*time.Second),
	)
	if err != nil {
		panic(err)
	}

	// NEED TO HANDLE THIS WITHOUT A SLEEP IN SOME WAY
	time.Sleep(15 * time.Second)

	// Forward WebDriverAgent to host container
	err = retry.Do(
		func() error {
			err := forwardPort(8100, 8100)
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(2*time.Second),
	)
	if err != nil {
		panic(err)
	}

	// Forward WebDriverAgent mjpeg stream to host container
	err = retry.Do(
		func() error {
			err := forwardPort(9100, 9100)
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(2*time.Second),
	)
	if err != nil {
		panic(err)
	}

	err = updateWDA()
	if err != nil {
		fmt.Println("Could not update WebDriverAgent stream settings, err: " + err.Error())
	}

	// Start Appium server
	go startAppium()
}

// Start usbmuxd service after starting the container
func startUsbmuxd() {
	prg := "usbmuxd"
	arg1 := "-U"
	arg2 := "usbmux"
	arg3 := "-f"

	// Build the usbmuxd command
	cmd := exec.Command(prg, arg1, arg2, arg3)

	// Run the command to start usbmuxd
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

// Mount the developer disk images downloading them automatically in /opt/DeveloperDiskImages
func mountDeveloperImage() error {
	err := imagemounter.FixDevImage(config.Device, "/opt/DeveloperDiskImages")
	if err != nil {
		return err
	}

	return nil
}

// Pair the device which is expected to be supervised
func pairDevice() error {
	p12, err := os.ReadFile("/opt/supervision.p12")
	if err != nil {
		return err
	}

	err = ios.PairSupervised(config.Device, p12, config.SupervisionPassword)
	if err != nil {
		return err
	}

	return nil
}

type appiumCapabilities struct {
	UDID                  string `json:"appium:udid"`
	WdaMjpegPort          string `json:"appium:mjpegServerPort"`
	ClearSystemFiles      string `json:"appium:clearSystemFiles"`
	WdaURL                string `json:"appium:webDriverAgentUrl"`
	PreventWdaAttachments string `json:"appium:preventWDAAttachments"`
	SimpleIsVisibleCheck  string `json:"appium:simpleIsVisibleCheck"`
	WdaLocalPort          string `json:"appium:wdaLocalPort"`
	PlatformVersion       string `json:"appium:platformVersion"`
	AutomationName        string `json:"appium:automationName"`
	PlatformName          string `json:"platformName"`
	DeviceName            string `json:"appium:deviceName"`
	WdaLaunchTimeout      string `json:"appium:wdaLaunchTimeout"`
	WdaConnectionTimeout  string `json:"appium:wdaConnectionTimeout"`
}

// Start the Appium server for the device
func startAppium() {
	capabilities1 := appiumCapabilities{
		UDID:                  config.UDID,
		WdaURL:                "http://localhost:8100",
		WdaMjpegPort:          "9100",
		WdaLocalPort:          "8100",
		WdaLaunchTimeout:      "120000",
		WdaConnectionTimeout:  "240000",
		ClearSystemFiles:      "false",
		PreventWdaAttachments: "true",
		SimpleIsVisibleCheck:  "false",
		AutomationName:        "XCUITest",
		PlatformName:          "iOS",
		DeviceName:            config.DeviceName,
	}
	capabilitiesJson, err := json.Marshal(capabilities1)
	if err != nil {
		panic(errors.New("Could not marshal Appium capabilities json, err: " + err.Error()))
	}

	// We are using /bin/bash -c here because os.exec does not invoke the system shell and `nvm` is not sourced in the container
	// should find a better solution in the future
	cmd := exec.Command("/bin/bash", "-c", "appium -p 4723 --log-timestamp --allow-cors --session-override --default-capabilities '"+string(capabilitiesJson)+"'")
	fmt.Println(cmd)

	outfile, err := os.Create("/opt/logs/appium.log")
	if err != nil {
		panic(err)
	}
	defer outfile.Close()
	cmd.Stdout = outfile
	cmd.Stderr = outfile

	err = cmd.Run()
	if err != nil {
		panic(err)
	}
}

// Install WebDriverAgent on the device
func prepareWDA() error {
	err := InstallAppWithDevice(config.Device, "WebDriverAgent.ipa")
	if err != nil {
		return err
	}

	go startWDA()
	return nil
}

// Start the WebDriverAgent on the device
func startWDA() {
	fmt.Println("Starting WDA")
	prg := "ios"
	arg1 := "runwda"
	arg2 := "--bundleid=" + config.BundleID
	arg3 := "--testrunnerbundleid=" + config.BundleID
	arg4 := "--xctestconfig=WebDriverAgentRunner.xctest"
	arg5 := "--udid=" + config.UDID

	cmd := exec.Command(prg, arg1, arg2, arg3, arg4, arg5)

	outfile, err := os.Create("/opt/logs/wda.log")
	if err != nil {
		panic(err)
	}
	defer outfile.Close()

	cmd.Stdout = outfile
	cmd.Stderr = outfile

	err = cmd.Run()
	if err != nil {
		fmt.Println(err.Error())
		panic(err)
	}
}

// Start WebDriverAgent directly using go-ios modules
func StartWDAInternal() error {

	go func() {
		err := testmanagerd.RunXCUIWithBundleIdsCtx(nil, config.BundleID,
			config.TestRunnerBundleID,
			config.XCTestConfig,
			config.Device,
			[]string{},
			[]string{"USE_PORT=" + config.WdaPort, "MJPEG_SERVER_PORT=" + config.WdaMjpegPort})

		if err != nil {
			log.WithFields(log.Fields{
				"event": "run_wda",
			}).Error("Failed running wda. Error: " + err.Error())
			panic(err)
		}
	}()

	return nil
}

// Forward a port from device to container using go-ios
func forwardPort(hostPort uint16, devicePort uint16) error {
	err := forward.Forward(config.Device, hostPort, devicePort)
	if err != nil {
		return err
	}

	return nil
}

// Create a new WebDriverAgent session and update stream settings
func updateWDA() error {
	fmt.Println("Updating WDA session")
	sessionID, err := createWDASession()
	if err != nil {
		return err
	}

	err = updateWdaStreamSettings(sessionID)
	if err != nil {
		return err
	}

	return nil
}

// Update WebDriverAgent stream settings
func updateWdaStreamSettings(sessionID string) error {
	requestString := `{"settings": {"mjpegServerFramerate": 30, "mjpegServerScreenshotQuality": 50, "mjpegScalingFactor": 100}}`

	response, err := http.Post("http://localhost:8100/session/"+sessionID+"/appium/settings", "application/json", strings.NewReader(requestString))
	if err != nil {
		return err
	}

	if response.StatusCode != 200 {
		return errors.New("Could not successfully update WDA stream settings")
	}

	return nil
}

// Create a new WebDriverAgent session
func createWDASession() (string, error) {
	requestString := `{
		"capabilities": {
			"firstMatch": [
				{
					"arguments": [],
					"environment": {},
					"eventloopIdleDelaySec": 0,
					"shouldWaitForQuiescence": true,
					"shouldUseTestManagerForVisibilityDetection": false,
					"maxTypingFrequency": 60,
					"shouldUseSingletonTestManager": true,
					"shouldTerminateApp": true,
					"forceAppLaunch": true,
					"useNativeCachingStrategy": true,
					"forceSimulatorSoftwareKeyboardPresence": false
				}
			],
			"alwaysMatch": {}
		}
	}`

	response, err := http.Post("http://localhost:8100/session", "application/json", strings.NewReader(requestString))
	if err != nil {
		return "", err
	}

	responseBody, _ := io.ReadAll(response.Body)

	var responseJson map[string]interface{}
	err = json.Unmarshal(responseBody, &responseJson)
	if err != nil {
		return "", err
	}

	if responseJson["sessionId"] == "" {
		if err != nil {
			return "", errors.New("Could not get `sessionId` while creating a new WebDriverAgent session")
		}
	}

	return fmt.Sprintf("%v", responseJson["sessionId"]), nil
}