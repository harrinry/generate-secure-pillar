package sls

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Everbridge/generate-secure-pillar/pki"
	yaml "github.com/esilva-everbridge/yaml"
	"github.com/sirupsen/logrus"
	yamlv2 "gopkg.in/yaml.v2"
)

// pgpHeader header const
const pgpHeader = "-----BEGIN PGP MESSAGE-----"

// Encrypt action
const Encrypt = "encrypt"

// Decrypt action
const Decrypt = "decrypt"

// Validate action (keys)
const Validate = "validate"

// Rotate action
const Rotate = "rotate"

var logger *logrus.Logger

// Sls sls data
type Sls struct {
	FilePath       string
	Yaml           *yaml.Yaml
	Pki            *pki.Pki
	IsInclude      bool
	EncryptionPath string
	KeyMap         map[string]interface{}
	Error          error
}

// New returns a Sls object
func New(filePath string, p pki.Pki, encPath string) Sls {
	logger = logrus.New()

	s := Sls{filePath, yaml.New(), &p, false, encPath, map[string]interface{}{}, nil}
	if len(filePath) > 0 {
		err := s.ReadSlsFile()
		if err != nil {
			logger.Errorf("init error for %s: %s", s.FilePath, err)
			s.Error = err
		}
	}

	return s
}

// ReadBytes loads YAML from a []byte
func (s *Sls) ReadBytes(buf []byte) error {
	s.Yaml = yaml.New()

	reader := strings.NewReader(string(buf))

	err := s.ScanForIncludes(reader)
	if err != nil {
		s.IsInclude = true
		logger.Warnf("%s", err)
	}

	return yamlv2.Unmarshal(buf, &s.Yaml.Values)
}

// ScanForIncludes looks for include statements in the given io.Reader
func (s *Sls) ScanForIncludes(reader io.Reader) error {
	// Splits on newlines by default.
	scanner := bufio.NewScanner(reader)

	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		txt := scanner.Text()
		if strings.Contains(txt, "include:") {
			return fmt.Errorf("%s contains include directives", shortFileName(s.FilePath))
		}
	}
	return scanner.Err()
}

// ReadSlsFile open and read a yaml file, if the file has include statements
// we throw an error as the YAML parser will try to act on the include directives
func (s *Sls) ReadSlsFile() error {
	if len(s.FilePath) == 0 {
		return fmt.Errorf("no file path given")
	}

	if s.FilePath == os.Stdout.Name() {
		return nil
	}

	if _, statErr := os.Stat(s.FilePath); os.IsNotExist(statErr) {
		dir := filepath.Dir(s.FilePath)
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			return err
		}
		_, err = os.OpenFile(s.FilePath, os.O_RDONLY|os.O_CREATE, 0600)
		if err != nil {
			return err
		}
	}

	fullPath, err := filepath.Abs(s.FilePath)
	if err != nil {
		return err
	}

	var buf []byte
	buf, err = ioutil.ReadFile(fullPath)
	if err != nil {
		return err
	}

	return s.ReadBytes(buf)
}

// WriteSlsFile writes a buffer to the specified file
// If the outFilePath is not stdout an INFO string will be printed to stdout
func WriteSlsFile(buffer bytes.Buffer, outFilePath string) (int, error) {
	fullPath, err := filepath.Abs(outFilePath)
	if err != nil {
		fullPath = outFilePath
	}

	stdOut := false
	if fullPath == os.Stdout.Name() {
		stdOut = true
	}

	// check that the path exists, create it if not
	if !stdOut {
		dir := filepath.Dir(fullPath)
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return buffer.Len(), fmt.Errorf("error creating sls path: %s", err)
		}
	}

	var byteCount int
	if stdOut {
		byteCount, err = fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", buffer.String()))
	} else {
		byteCount, err = atomicWrite(fullPath, buffer)
	}

	if !stdOut && err == nil {
		shortFile := shortFileName(outFilePath)
		logger.Infof("wrote out to file: '%s'", shortFile)
	}

	return byteCount, err
}

func atomicWrite(fullPath string, buffer bytes.Buffer) (int, error) {
	_, name := path.Split(fullPath)
	f, err := ioutil.TempFile("", fmt.Sprintf("gsp-%s", name))
	if err != nil {
		return 0, err
	}
	byteCount, err := f.Write(buffer.Bytes())
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if permErr := os.Chmod(f.Name(), 0600); err == nil {
		err = permErr
	}
	if err == nil {
		err = copyFile(f.Name(), fullPath)
	}
	if err != nil {
		return byteCount, err
	}

	if _, statErr := os.Stat(f.Name()); !os.IsNotExist(statErr) {
		err = os.Remove(f.Name())
	}

	return byteCount, err
}

func copyFile(src string, dst string) error {
	srcStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	fsrc, err := os.Open(src)
	if err != nil {
		return err
	}

	fdst, err := os.Create(dst)
	if err != nil {
		return err
	}

	size, err := io.Copy(fdst, fsrc)
	if err != nil {
		return err
	}
	if size != srcStat.Size() {
		return fmt.Errorf("%s: %d/%d copied", src, size, srcStat.Size())
	}

	err = fsrc.Close()
	if err != nil {
		return fdst.Close()
	}
	return err
}

// FormatBuffer returns a formatted .sls buffer with the gpg renderer line
func (s *Sls) FormatBuffer(action string) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	var out []byte
	var err error
	var data map[string]interface{}

	if action != Validate {
		data = s.Yaml.Values
	} else {
		data = s.KeyMap
	}

	if len(data) == 0 {
		return buffer, fmt.Errorf("%s has no values to format", s.FilePath)
	}

	out, err = yamlv2.Marshal(data)
	if err != nil {
		return buffer, fmt.Errorf("%s format error: %s", s.FilePath, err)
	}

	if action != Validate {
		buffer.WriteString("#!yaml|gpg\n\n")
	}
	_, err = buffer.WriteString(string(out))

	return buffer, err
}

// ProcessYaml encrypts elements matching keys specified on the command line
func (s *Sls) ProcessYaml(secretNames []string, secretValues []string) error {
	var err error

	for index := 0; index < len(secretNames); index++ {
		cipherText := ""
		if index >= 0 && index < len(secretValues) {
			cipherText, err = s.Pki.EncryptSecret(secretValues[index])
			if err != nil {
				return err
			}
		}
		err = s.SetValueFromPath(secretNames[index], cipherText)
		if err != nil {
			return err
		}
	}

	return err
}

// GetValueFromPath returns the value from a path string
func (s *Sls) GetValueFromPath(path string) interface{} {
	parts := strings.Split(path, ":")

	args := make([]interface{}, len(parts))
	for i := 0; i < len(parts); i++ {
		args[i] = parts[i]
	}
	results := s.Yaml.Get(args...)
	return results
}

// SetValueFromPath returns the value from a path string
func (s *Sls) SetValueFromPath(path string, value string) error {
	parts := strings.Split(path, ":")

	// construct the args list
	args := make([]interface{}, len(parts)+1)
	for i := 0; i < len(parts); i++ {
		args[i] = parts[i]
	}
	args[len(args)-1] = value
	err := s.Yaml.Set(args...)
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", err)
}

// PerformAction takes an action string (encrypt or decrypt)
// and applies that action on all items
func (s *Sls) PerformAction(action string) (bytes.Buffer, error) {
	var err error
	var buf bytes.Buffer

	if validAction(action) {
		var stuff = make(map[string]interface{})

		for key := range s.Yaml.Values {
			if s.EncryptionPath != "" {
				vals := s.GetValueFromPath(key)
				if s.EncryptionPath == key {
					stuff[key], err = s.ProcessValues(vals, action)
					if err != nil {
						return buf, err
					}
				} else {
					stuff[key] = vals
				}
			} else {
				vals := s.GetValueFromPath(key)
				stuff[key], err = s.ProcessValues(vals, action)
				if err != nil {
					return buf, err
				}
			}
		}
		if action != Validate {
			// replace the values in the Yaml object
			s.Yaml.Values = stuff
		} else {
			s.KeyMap = stuff
		}
	}

	return s.FormatBuffer(action)
}

// ProcessValues will encrypt or decrypt given values
func (s *Sls) ProcessValues(vals interface{}, action string) (interface{}, error) {
	var res interface{}
	var err error

	if vals == nil {
		return res, nil
	}

	vtype := reflect.TypeOf(vals).Kind()
	switch vtype {
	case reflect.Slice:
		return s.doSlice(vals, action)
	case reflect.Map:
		return s.doMap(vals.(map[interface{}]interface{}), action)
	case reflect.String:
		return s.doString(vals, action)
	}

	return res, err
}

func (s *Sls) doSlice(vals interface{}, action string) (interface{}, error) {
	var things []interface{}

	if vals == nil {
		return things, nil
	}

	for _, item := range vals.([]interface{}) {
		var thing interface{}
		vtype := reflect.TypeOf(item).Kind()

		switch vtype {
		case reflect.Slice:
			sliceStuff, err := s.doSlice(item, action)
			if err != nil {
				return vals, err
			}
			things = append(things, sliceStuff)
		case reflect.Map:
			thing = item
			mapStuff, err := s.doMap(thing.(map[interface{}]interface{}), action)
			if err != nil {
				return vals, err
			}
			things = append(things, mapStuff)
		case reflect.String:
			thing, err := s.doString(item, action)
			if err != nil {
				return vals, err
			}
			things = append(things, thing)
		}
	}

	return things, nil
}

func (s *Sls) doMap(vals map[interface{}]interface{}, action string) (map[interface{}]interface{}, error) {
	var ret = make(map[interface{}]interface{})
	var err error

	for key, val := range vals {
		if val == nil {
			return ret, err
		}

		vtype := reflect.TypeOf(val).Kind()
		switch vtype {
		case reflect.Slice:
			ret[key], err = s.doSlice(val, action)
		case reflect.Map:
			ret[key], err = s.doMap(val.(map[interface{}]interface{}), action)
		case reflect.String:
			ret[key], err = s.doString(val, action)
		}
	}

	return ret, err
}

func (s *Sls) doString(val interface{}, action string) (string, error) {
	var err error

	strVal := val.(string)
	switch action {
	case Decrypt:
		strVal, err = s.decryptVal(strVal)
		if err != nil {
			return val.(string), err
		}
	case Encrypt:
		if !isEncrypted(strVal) {
			strVal, err = s.Pki.EncryptSecret(strVal)
			if err != nil {
				return val.(string), err
			}
		}
	case Validate:
		strVal, err = s.keyInfo(strVal)
		if err != nil {
			return val.(string), err
		}
	case Rotate:
		strVal, err = s.rotateVal(strVal)
		if err != nil {
			return val.(string), err
		}
	}

	return strVal, err
}

func (s *Sls) rotateVal(strVal string) (string, error) {
	strVal, err := s.decryptVal(strVal)
	if err != nil {
		return strVal, err
	}
	return s.Pki.EncryptSecret(strVal)
}

func isEncrypted(str string) bool {
	return strings.Contains(str, pgpHeader)
}

func (s *Sls) keyInfo(val string) (string, error) {
	if !isEncrypted(val) {
		return val, fmt.Errorf("value is not encrypted")
	}

	tmpfile, err := ioutil.TempFile("", "gsp-")
	if err != nil {
		return val, fmt.Errorf("keyInfo: %s", err)
	}

	if _, err = tmpfile.Write([]byte(val)); err != nil {
		return val, fmt.Errorf("keyInfo: %s", err)
	}

	keyInfo, err := s.Pki.KeyUsedForEncryptedFile(tmpfile.Name())
	if err != nil {
		return val, fmt.Errorf("keyInfo: %s", err)
	}

	if err = tmpfile.Close(); err != nil {
		return val, fmt.Errorf("keyInfo: %s", err)
	}
	if err = os.Remove(tmpfile.Name()); err != nil {
		return val, fmt.Errorf("keyInfo: %s", err)
	}

	return keyInfo, nil
}

func (s *Sls) decryptVal(strVal string) (string, error) {
	var plainText string

	if isEncrypted(strVal) {
		var err error
		plainText, err = s.Pki.DecryptSecret(strVal)
		if err != nil {
			return strVal, fmt.Errorf("error decrypting value: %s", err)
		}
	} else {
		return strVal, nil
	}

	return plainText, nil
}

func validAction(action string) bool {
	return action == Encrypt || action == Decrypt || action == Validate || action == Rotate
}

func shortFileName(file string) string {
	pwd, err := os.Getwd()
	if err != nil {
		logger.Warnf("%s", err)
		return file
	}
	return strings.Replace(file, pwd+"/", "", 1)
}
