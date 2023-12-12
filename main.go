package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
)

var ArchitecturesMap = map[string]string{
	"amd64": "x64",
}

type AvailableReleaseResponse struct {
	AvailableLtsReleases     []uint `json:"available_lts_releases"`
	AvailableReleases        []uint `json:"available_releases"`
	MostRecentFeatureRelease uint   `json:"most_recent_feature_release"`
	MostRecentFeatureVersion uint   `json:"most_recent_feature_version"`
	MostRecentLts            uint   `json:"most_recent_lts"`
	TipVersion               uint   `json:"tip_version"`
}

func main() {

	hipoHome, done := PrepareHipoHome()

	if !done {
		return
	}

	mostRecentJavaRelease, done := GetLatestJavaRelease()

	if !done {
		return
	}

	var arch = ArchitecturesMap[runtime.GOARCH]
	var osName = runtime.GOOS

	done = DownloadJava(mostRecentJavaRelease, osName, arch, hipoHome)

	if !done {
		return
	}
}

func PrepareHipoHome() (string, bool) {

	homeDir, err := os.UserHomeDir()

	if err != nil {
		fmt.Println("Error:", err)
		return "", false
	}

	var hipoHomeDir = homeDir + "/.hipo"

	err = os.MkdirAll(hipoHomeDir, 0755)

	if err != nil {
		fmt.Println("Error:", err)
		return "", false
	}

	return hipoHomeDir, true
}

func DownloadJava(release uint, osName string, arch string, hipoHome string) bool {

	url := fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jdk/hotspot/normal/eclipse?project=jdk", release, osName, arch)
	resp, err := http.Get(url)

	if err != nil {
		fmt.Println("Error:", err)
		return false
	}

	out, err := os.Create(hipoHome + "/jdk.tar.gz") // create file

	if err != nil {
		fmt.Println("Error:", err)
		return false
	}

	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			fmt.Println("Error:", err)
		}
	}(out)

	_, err = io.Copy(out, resp.Body) // save binary to file
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}

	return true
}

func GetLatestJavaRelease() (uint, bool) {
	resp, err := http.Get("https://api.adoptium.net/v3/info/available_releases")

	if err != nil {
		fmt.Println("Error:", err)
		return 0, false
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		fmt.Println("Error:", err)
		return 0, false
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error:", err)
		}
	}(resp.Body)

	var response AvailableReleaseResponse

	err = json.Unmarshal(body, &response)

	if err != nil {
		fmt.Println("Error:", err)
		return 0, false
	}

	return response.MostRecentFeatureRelease, true
}
