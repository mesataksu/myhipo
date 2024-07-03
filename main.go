package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	if !isHipoExists() {
		InitHipo()
	}

	if len(os.Args) == 1 {

		fmt.Printf("Usage:\nhipo group:artifact:version\n")

	} else if len(os.Args) == 2 {

		destPath, done := downloadFile(os.Args[1])

		if done {
			executeFile(destPath)
		}
	}

}

func isHipoExists() bool {

	homeDir, err := os.UserHomeDir()

	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	var javaDir = filepath.Join(homeDir, ".hipo", "jre")

	if _, err := os.Stat(javaDir); os.IsNotExist(err) {
		return false
	}

	_, found := findJava(javaDir)

	return found

}

func findJava(javaDir string) (string, bool) {

	entries, err := os.ReadDir(javaDir)
	if err != nil {
		fmt.Println("Error reading jre directory:", err)
		os.Exit(1)
	}

	//checks all files in the parent jre directory to find bin directory
	for _, entry := range entries {

		if entry.IsDir() {

			binDir := filepath.Join(javaDir, entry.Name(), "bin")

			if _, err := os.Stat(binDir); os.IsNotExist(err) {
				continue
			}

			binEntries, err := os.ReadDir(binDir)

			if err != nil {
				fmt.Println("Error reading bin directory:", err)
				os.Exit(1)
			}

			//checks all files in the bin directory to find java executable
			for _, binEntry := range binEntries {

				fileName := filepath.Join(binDir, binEntry.Name())

				baseName := filepath.Base(fileName)

				if baseName == "java" || baseName == "java.exe" {
					return fileName, true
				}
			}

		}
	}

	return "", false
}

func InitHipo() {

	hipoHome, done := PrepareHipoHome()

	if !done {
		os.Exit(1)
	}

	mostRecentJavaRelease, done := GetLatestJavaRelease()

	if !done {
		os.Exit(1)
	}

	var arch = ArchitecturesMap[runtime.GOARCH]
	var osName = runtime.GOOS

	done = DownloadJava(mostRecentJavaRelease, osName, arch, hipoHome)

	if !done {
		os.Exit(1)
	}
}

func downloadFile(Args string) (string, bool) {

	parts := strings.Split(Args, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		fmt.Println("Invalid coordinate format. Use <group:artifact:version>")
		return "", false
	}

	// goes to link
	groupID := parts[0]
	artifactID := parts[1]
	version := parts[2]

	groupPath := strings.ReplaceAll(groupID, ".", "/")
	artifactFilename := fmt.Sprintf("%s-%s.jar", artifactID, version)
	url := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s", groupPath, artifactID, version, artifactFilename)

	homeDir, err := os.UserHomeDir()

	if err != nil {
		fmt.Println("Error:", err)
		return "", false
	}

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("failed to make GET request: %v", err)
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Package not found in maven repository\n")
		return "", false
	}

	destDir := filepath.Join(homeDir, ".hipo", "cache", groupPath, artifactID, version)

	//copies the downloaded content into the new file in the destDir
	return copyFile(destDir, artifactFilename, resp.Body), true

}

func copyFile(destDir string, artifactFilename string, respBody io.ReadCloser) string {

	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		return ""
	}

	destPath := filepath.Join(destDir, artifactFilename)

	out, err := os.Create(destPath)
	if err != nil {
		fmt.Printf("failed to create file: %v", err)
		return ""
	}
	defer out.Close()

	_, err = io.Copy(out, respBody)
	if err != nil {
		fmt.Printf("failed to copy response body to file: %v", err)
		return ""
	}

	return destPath
}

func executeFile(jarFilePath string) {

	homeDir, err := os.UserHomeDir()

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	jreParentDir := filepath.Join(homeDir, ".hipo", "jre")

	// Path to the java executable
	javaExecPath, found := findJava(jreParentDir)

	if !found {
		fmt.Println("Java file is not found")
		return
	}

	if runtime.GOOS != "windows" {
		err = os.Chmod(javaExecPath, 0755)
		if err != nil {
			fmt.Println("Error setting execute permission:", err)
			return
		}
	}

	cmd := exec.Command(javaExecPath, "-jar", jarFilePath)

	// Set the command's standard output and standard error
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Println("Error running the Java command:", err)
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

	url := fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jre/hotspot/normal/eclipse?project=jdk",
		release, osName, arch)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error: failed to make GET request:", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
		return false
	}

	var hipoJreDir = hipoHome + "/jre"

	//creates the destination directory
	err = os.MkdirAll(hipoJreDir, 0755)
	if err != nil {
		fmt.Println("Error: failed to create directory:", err)
		return false
	}

	if osName == "windows" {
		// Extract the zip file contents to the destination directory
		if err := ExtractZip(resp.Body, hipoHome+"/jre"); err != nil {
			fmt.Println("Error extracting ZIP file:", err)
			return false
		}
	} else {
		// Extract the tar file contents to the destination directory
		if err := ExtractTarGz(resp.Body, hipoHome+"/jre"); err != nil {
			fmt.Println("Error extracting TarGz file:", err)
			return false
		}
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

func ExtractZip(reader io.Reader, destination string) error {

	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	r, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(destination, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func ExtractTarGz(reader io.Reader, destination string) error {

	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		fpath := filepath.Join(destination, header.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(fpath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return err
			}

			outFile, err := os.Create(fpath)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}

			outFile.Close()
		}

	}

	return nil
}
