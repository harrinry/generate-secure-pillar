package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

func writeSlsFile(buffer bytes.Buffer, outFilePath string) {
	fullPath, err := filepath.Abs(outFilePath)
	if err != nil {
		fullPath = outFilePath
	}

	stdOut := false
	if fullPath == os.Stdout.Name() {
		stdOut = true
	}

	err = ioutil.WriteFile(fullPath, buffer.Bytes(), 0644)
	if err != nil {
		logger.Fatal("error writing sls file: ", err)
	}
	if !stdOut {
		logger.Printf("Wrote out to file: '%s'\n", outFilePath)
	}
}

func readSlsFile(slsPath string) (SlsData, error) {
	filename, err := filepath.Abs(slsPath)
	if err != nil {
		logger.Fatal(err)
	}
	var slsData = make(SlsData)
	var yamlData []byte

	if _, err = os.Stat(filename); !os.IsNotExist(err) {
		yamlData, err = ioutil.ReadFile(filename)
		if err != nil {
			logger.Fatal("error reading YAML file: ", err)
		}

		err = yaml.Unmarshal(yamlData, &slsData)
		if err != nil {
			logger.Printf(fmt.Sprintf("Skipping %s: %s\n", filename, err))
		}
	}

	return slsData, err
}

func findSlsFiles(searchDir string) ([]string, int) {
	searchDir, _ = filepath.Abs(searchDir)
	fileList := []string{}
	err := filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if !f.IsDir() && strings.Contains(f.Name(), ".sls") {
			fileList = append(fileList, path)
		}
		return nil
	})
	if err != nil {
		logger.Fatal("error walking file path: ", err)
	}

	return fileList, len(fileList)
}

func pillarBuffer(filePath string, all bool) bytes.Buffer {
	err := checkForFile(filePath)
	if err != nil {
		logger.Fatal(err)
	}
	filePath, err = filepath.Abs(filePath)
	if err != nil {
		logger.Fatal(err)
	}

	var buffer bytes.Buffer
	var cipherText string
	pillar, err := readSlsFile(filePath)
	if err != nil {
		logger.Fatal(err)
	}
	dataChanged := false

	if all {
		if keyExists(pillar, "secure_vars") && len(pillar["secure_vars"].(SlsData)) != 0 {
			pillar, dataChanged = pillarRange(pillar)
		} else {
			logger.Infof(fmt.Sprintf("%s has no secure_vars element", filePath))
		}
	} else {
		cipherText = encryptSecret(secretsString)
		if keyExists(pillar, "secure_vars") {
			pillar["secure_vars"].(SlsData)[secretName] = cipherText
		} else {
			pillar[secretName] = cipherText
		}
		dataChanged = true
	}

	if !dataChanged {
		return buffer
	}

	return formatBuffer(pillar)
}

func pillarRange(pillar SlsData) (SlsData, bool) {
	var dataChanged = false
	for k, v := range pillar["secure_vars"].(SlsData) {
		if !strings.Contains(v.(string), pgpHeader) {
			cipherText := encryptSecret(v.(string))
			pillar["secure_vars"].(SlsData)[k] = cipherText
			dataChanged = true
		}
	}
	return pillar, dataChanged
}

func plainTextPillarBuffer(inFile string) bytes.Buffer {
	inFile, _ = filepath.Abs(inFile)
	pillar, err := readSlsFile(inFile)
	if err != nil {
		logger.Fatal(err)
	}

	if pillar["secure_vars"] != nil {
		for k, v := range pillar["secure_vars"].(SlsData) {
			if strings.Contains(v.(string), pgpHeader) {
				plainText := decryptSecret(v.(string))
				pillar["secure_vars"].(SlsData)[k] = plainText
			}
		}
	}

	return formatBuffer(pillar)
}

func formatBuffer(pillar SlsData) bytes.Buffer {
	var buffer bytes.Buffer

	yamlBytes, err := yaml.Marshal(pillar)
	if err != nil {
		logger.Fatalf("error marshalling YAML: %s", err)
	}

	buffer.WriteString("#!yaml|gpg\n\n")
	buffer.WriteString(string(yamlBytes))

	return buffer
}

func checkForFile(filePath string) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		logger.Fatalf("cannot stat %s: %s", filePath, err)
	}
	switch mode := fi.Mode(); {
	case mode.IsRegular():
		return nil
	case mode.IsDir():
		logger.Fatalf("%s is a directory", filePath)
	}

	return err
}

func writeSlsData(file string) {
	pillar, err := readSlsFile(file)
	if err != nil {
		logger.Fatalf("error reading sls file: %s", err)
	}
	if len(pillar["secure_vars"].(SlsData)) > 0 {
		buffer := pillarBuffer(file, true)
		writeSlsFile(buffer, fmt.Sprintf("%s.new", file))
	}
}

func keyExists(decoded map[interface{}]interface{}, key string) bool {
	val, ok := decoded[key]
	return ok && val != nil
}