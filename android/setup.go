package android_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/codeskyblue/go-sh"
	"github.com/shamanec/GADS-docker-server/config"
)

type appiumCapabilities struct {
	UDID           string `json:"appium:udid"`
	AutomationName string `json:"appium:automationName"`
	PlatformName   string `json:"platformName"`
	DeviceName     string `json:"appium:deviceName"`
}

func SetupDevice() {
	fmt.Println("INFO: Device setup")

	// Check if device is available to adb
	err := retry.Do(
		func() error {
			err := checkDeviceAvailable()
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(3*time.Second),
	)
	if err != nil {
		panic(err)
	}

	err = UseGadsStream()
	if err != nil {
		panic(err)
	}

	// Start getting gads stream after service was started and forwarded to host container
	go ConnectGadsStreamWS()

	// adb shell am instrument -w -e debug false com.shamanec.stream.test/androidx.test.runner.AndroidJUnitRunner

	//Try to forward instrumentation socket to host container
	// err = retry.Do(
	// 	func() error {
	// 		err := forwardInstrumentation()
	// 		if err != nil {
	// 			fmt.Println("This is error from forward instrumentation")
	// 			return err
	// 		}
	// 		return nil
	// 	},
	// 	retry.Attempts(3),
	// 	retry.Delay(3*time.Second),
	// )
	// if err != nil {
	// 	panic(err)
	// }

	// Start the instrumentation test
	//go startInstrumentation()

	// Start the Appium server
	go startAppium()
}

func UseGadsStream() error {
	streamAvailable := false

	// Check if the gads-stream service is already running on the device
	// to avoid steps to install, permit and run it
	err := retry.Do(
		func() error {
			isAvailable, err := checkGadsStreamServiceRunning()
			if err != nil {
				return err
			}
			streamAvailable = isAvailable
			return nil
		},
		retry.Attempts(3),
		retry.Delay(3*time.Second),
	)
	if err != nil {
		return err
	}

	if !streamAvailable {
		// Installing gads-stream.apk
		err = retry.Do(
			func() error {
				err := installGadsStream()
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(3),
			retry.Delay(3*time.Second),
		)
		if err != nil {
			return err
		}

		// Add recording permissions to gads-stream app
		err = retry.Do(
			func() error {
				err := addGadsStreamRecordingPermissions()
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(3),
			retry.Delay(3*time.Second),
		)
		if err != nil {
			return err
		}

		// Start the gads-stream app
		err = retry.Do(
			func() error {
				err := startGadsStreamApp()
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(3),
			retry.Delay(3*time.Second),
		)
		if err != nil {
			return err
		}

		// Press the Home button to hide the gads-stream app activity
		err = retry.Do(
			func() error {
				err := pressHomeButton()
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(3),
			retry.Delay(3*time.Second),
		)
		if err != nil {
			return err
		}
	}

	time.Sleep(5 * time.Second)

	//Try to forward gads-stream to host container
	err = retry.Do(
		func() error {
			err := forwardGadsStream()
			if err != nil {
				fmt.Println("This is error from forwarding gads-stream")
				return err
			}
			return nil
		},
		retry.Attempts(3),
		retry.Delay(3*time.Second),
	)
	if err != nil {
		return err
	}

	return nil
}

// Check if the Android device is available to adb
func checkDeviceAvailable() error {
	fmt.Println("INFO: Checking if device is available to adb")

	output, err := sh.Command("adb", "devices").Output()
	if err != nil {
		return errors.New("Could not execute `adb devices`, err: " + err.Error())
	}

	// Check if we got the device UDID in the list of `adb devices`
	if strings.Contains(string(output), config.UDID) {
		return nil
	}

	return errors.New("Device with UDID=" + config.UDID + " was not available to adb")
}

func checkGadsStreamServiceRunning() (bool, error) {
	fmt.Println("INFO: Checking if gads-stream is installed and service is running")

	output, err := sh.Command("adb", "shell", "dumpsys", "activity", "services", "com.shamanec.stream/.ScreenCaptureService").Output()
	if err != nil {
		return false, errors.New("Could not execute adb shell dumpsys for the gads-stream service, err: " + err.Error())
	}

	// If command returned "(nothing)" then the service is not running
	if strings.Contains(string(output), "(nothing)") {
		fmt.Println(string(output))
		return false, nil
	}

	return true, nil
}

// Install gads-stream.apk on the device
func installGadsStream() error {
	fmt.Println("INFO: Installing gads-stream.apk on the device")

	err := sh.Command("adb", "install", "-r", "/opt/gads-stream.apk").Run()
	if err != nil {
		return errors.New("Could not install gads-stream.apk, err: " + err.Error())
	}

	return nil
}

// Add recording permissions to gads-stream app to avoid popup on start
func addGadsStreamRecordingPermissions() error {
	fmt.Println("INFO: Adding recording permissions to gads-stream app")
	err := sh.Command("adb", "shell", "appops", "set", "com.shamanec.stream", "PROJECT_MEDIA", "allow").Run()
	if err != nil {
		return errors.New("Could not execute add permissions for recording to gads-stream app, err: " + err.Error())
	}

	return nil
}

// Start the gads-stream app using adb
func startGadsStreamApp() error {
	fmt.Println("INFO: Starting gads-stream app")
	err := sh.Command("adb", "shell", "am", "start", "-n", "com.shamanec.stream/com.shamanec.stream.ScreenCaptureActivity").Run()
	if err != nil {
		return errors.New("Could not start gads-streamm app, err: " + err.Error())
	}

	return nil
}

// Press the Home button using adb to hide the transparent gads-stream activity
func pressHomeButton() error {
	fmt.Println("INFO: Pressing Home button to hide the gads-stream activity")
	err := sh.Command("adb", "shell", "input", "keyevent", "KEYCODE_HOME").Run()
	if err != nil {
		return errors.New("Could press Home button successfully, err: " + err.Error())
	}

	return nil
}

// Forward gads-stream socket to the host container
func forwardGadsStream() error {
	fmt.Println("INFO: Forwarding gads-stream connection to tcp:1313")

	err := sh.Command("adb", "forward", "tcp:1313", "tcp:1991").Run()
	if err != nil {
		return err
	}

	return nil
}

func startInstrumentation() error {
	fmt.Println("INFO: Starting instrumentation")
	err := sh.Command("adb", "shell", "am", "instrument", "-w", "-e", "debug", "false", "com.shamanec.stream.test").Start()
	if err != nil {
		return errors.New("Could not start instrumentation successfully, err: " + err.Error())
	}

	return nil
}

// Forward instrumentation socket to the host container
func forwardInstrumentation() error {
	fmt.Println("INFO: Forwarding gads-stream connection to tcp:1313")

	err := sh.Command("adb", "forward", "tcp:1314", "tcp:1992").Run()
	if err != nil {
		return err
	}

	return nil
}

// Starts the Appium server on the device
func startAppium() {
	fmt.Println("INFO: Starting Appium server")

	// Create the Appium capabilities
	capabilities := appiumCapabilities{
		UDID:           config.UDID,
		AutomationName: "UiAutomator2",
		PlatformName:   "Android",
		DeviceName:     config.DeviceName,
	}
	// Marshal the capabilities into a json
	capabilitiesJson, err := json.Marshal(capabilities)
	if err != nil {
		panic(errors.New("Could not marshal Appium capabilities json, err: " + err.Error()))
	}

	// Create a json file for the capabilities
	capabilitiesFile, err := os.Create("/opt/capabilities.json")
	if err != nil {
		panic(err)
	}

	// Wrute the json byte slice to the json file created above
	_, err = capabilitiesFile.Write(capabilitiesJson)
	if err != nil {
		panic(err)
	}

	// Create file for the Appium logs
	outfile, err := os.Create("/opt/logs/appium.log")
	if err != nil {
		panic(err)
	}
	defer outfile.Close()

	// Create new shell session and redirect Stdout and Stderr to the Appium logs file
	session := sh.NewSession()
	session.Stdout = outfile
	session.Stderr = outfile

	// Start the Appium server with default cli arguments and using default capabilities from the file created above
	err = session.Command("appium", "-p", "4723", "--log-timestamp", "--allow-cors", "--allow-insecure", "chromedriver_autodownload", "--default-capabilities", "/opt/capabilities.json").Run()
	if err != nil {
		panic(err)
	}
}
