package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/Everbridge/generate-secure-pillar/pki"
	"github.com/Everbridge/generate-secure-pillar/sls"
	"github.com/Everbridge/generate-secure-pillar/utils"
	yaml "gopkg.in/yaml.v2"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// GSPProfile config data
type GSPProfile struct {
	Name           string `yaml:"name"`
	Default        bool   `yaml:"default"`
	DefaultKey     string `yaml:"default_key"`
	GnupgHome      string `yaml:"gnupg_home"`
	DefaultPubRing string `yaml:"default_pub_ring"`
	DefaultSecRing string `yaml:"default_sec_ring"`
}

// GSPConfig config data
type GSPConfig struct {
	Profiles []GSPProfile `yaml:"profiles"`
}

var logger = logrus.New()

var inputFilePath string
var outputFilePath = os.Stdout.Name()
var pgpKeyName string
var publicKeyRing = ""
var secretKeyRing = ""
var recurseDir string
var secretNames cli.StringSlice
var secretValues cli.StringSlice
var topLevelElement string
var yamlPath string
var updateInPlace bool
var pk pki.Pki

var defaultProfileName = ""
var defaultKeyName = ""
var defaultPubRing = "~/.gnupg/pubring.gpg"
var defaultSecRing = "~/.gnupg/secring.gpg"

var inputFlag = cli.StringFlag{
	Name:        "file, f",
	Value:       os.Stdin.Name(),
	Usage:       "input file (defaults to STDIN)",
	Destination: &inputFilePath,
}

var outputFlag = cli.StringFlag{
	Name:        "outfile, o",
	Value:       os.Stdout.Name(),
	Usage:       "output file (defaults to STDOUT)",
	Destination: &outputFilePath,
}

var fileFlags = []cli.Flag{
	inputFlag,
	outputFlag,
}

var secNamesFlag = cli.StringSliceFlag{
	Name:  "name, n",
	Usage: "secret name(s)",
	Value: &secretNames,
}

var secValsFlag = cli.StringSliceFlag{
	Name:  "value, s",
	Usage: "secret value(s)",
	Value: &secretValues,
}

var updateFlag = cli.BoolFlag{
	Name:        "update, u",
	Usage:       "update the input file",
	Destination: &updateInPlace,
}

var dirFlag = cli.StringFlag{
	Name:        "dir, d",
	Usage:       "recurse over all .sls files in the given directory",
	Destination: &recurseDir,
}

var appHelp = fmt.Sprintf(`%s
	CAVEAT: YAML files with include statements are not handled properly, so we skip them.
	
	EXAMPLES:
	# specify a config profile and create a new file
	$ generate-secure-pillar --profile dev create --name secret_name1 --value secret_value1 --name secret_name2 --value secret_value2 --outfile new.sls

	# create a new sls file
	$ generate-secure-pillar -k "Salt Master" create --name secret_name1 --value secret_value1 --name secret_name2 --value secret_value2 --outfile new.sls
	
	# add to the new file
	$ generate-secure-pillar -k "Salt Master" update --name new_secret_name --value new_secret_value --file new.sls
	
	# update an existing value
	$ generate-secure-pillar -k "Salt Master" update --name secret_name --value secret_value3 --file new.sls
	
	# encrypt all plain text values in a file
	$ generate-secure-pillar -k "Salt Master" encrypt all --file us1.sls --outfile us1.sls
	# or use --update flag
	$ generate-secure-pillar -k "Salt Master" encrypt all --file us1.sls --update
	
	# encrypt all plain text values in a file under the element 'secret_stuff'
	$ generate-secure-pillar -k "Salt Master" --element secret_stuff encrypt all --file us1.sls --outfile us1.sls
	
	# recurse through all sls files, encrypting all values
	$ generate-secure-pillar -k "Salt Master" encrypt recurse -d /path/to/pillar/secure/stuff
	
	# recurse through all sls files, decrypting all values (requires imported private key)
	$ generate-secure-pillar decrypt recurse -d /path/to/pillar/secure/stuff
	
	# decrypt a specific existing value (requires imported private key)
	$ generate-secure-pillar decrypt path --path "some:yaml:path" --file new.sls
	
	# decrypt all files and re-encrypt with given key (requires imported private key)
	$ generate-secure-pillar -k "New Salt Master Key" rotate -d /path/to/pillar/secure/stuff

	# show all PGP key IDs used in a file
	$ generate-secure-pillar keys all --file us1.sls

	# show all keys used in all files in a given directory
	$ generate-secure-pillar keys recurse -d /path/to/pillar/secure/stuff

	# show the PGP Key ID used for an element at a path in a file
	$ generate-secure-pillar keys path --path "some:yaml:path" --file new.sls

`, cli.AppHelpTemplate)

var appCommands = []cli.Command{
	{
		Name:    "create",
		Aliases: []string{"c"},
		Usage:   "create a new sls file",
		Action: func(c *cli.Context) error {
			setProfile()
			pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
			s := sls.New(outputFilePath, pk, topLevelElement)
			err := s.ProcessYaml(secretNames, secretValues)
			if err != nil {
				logger.Fatalf("create: %s", err)
			}
			buffer, err := s.FormatBuffer("")
			if err != nil {
				logger.Fatalf("create: %s", err)
			}
			_, err = sls.WriteSlsFile(buffer, outputFilePath)
			if err != nil {
				logger.Fatalf("create: %s", err)
			}
			return nil
		},
		Flags: []cli.Flag{
			outputFlag,
			secNamesFlag,
			secValsFlag,
		},
	},
	{
		Name:    "update",
		Aliases: []string{"u"},
		Usage:   "update the value of the given key in the given file",
		Action: func(c *cli.Context) error {
			setProfile()
			if inputFilePath != os.Stdin.Name() {
				outputFilePath = inputFilePath
			}
			pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
			s := sls.New(inputFilePath, pk, topLevelElement)
			err := s.ProcessYaml(secretNames, secretValues)
			if err != nil {
				logger.Fatal(err)
			}
			buffer, err := s.FormatBuffer("")
			if err != nil {
				logger.Fatal(err)
			}
			_, err = sls.WriteSlsFile(buffer, outputFilePath)
			if err != nil {
				logger.Fatal(err)
			}
			return nil
		},
		Flags: []cli.Flag{
			inputFlag,
			secNamesFlag,
			secValsFlag,
		},
	},
	{
		Name:    "encrypt",
		Aliases: []string{"e"},
		Usage:   "perform encryption operations",
		Action: func(c *cli.Context) error {
			return cli.ShowCommandHelp(c, "")
		},
		Subcommands: []cli.Command{
			{
				Name: "all",
				Flags: []cli.Flag{
					inputFlag,
					outputFlag,
					updateFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					if inputFilePath != os.Stdin.Name() && updateInPlace {
						outputFilePath = inputFilePath
					}
					buffer, err := s.PerformAction("encrypt")
					utils.SafeWrite(buffer, outputFilePath, err)
					return nil
				},
			},
			{
				Name: "recurse",
				Flags: []cli.Flag{
					dirFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					err := utils.ProcessDir(recurseDir, ".sls", "encrypt", outputFilePath, topLevelElement, pk)
					if err != nil {
						logger.Warnf("encrypt: %s", err)
					}
					return nil
				},
			},
			{
				Name: "path",
				Flags: []cli.Flag{
					inputFlag,
					cli.StringFlag{
						Name:        "path, p",
						Usage:       "YAML path to encrypt",
						Destination: &yamlPath,
					},
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					utils.PathAction(&s, yamlPath, "encrypt")

					return nil
				},
			},
		},
	},
	{
		Name:    "decrypt",
		Aliases: []string{"d"},
		Usage:   "perform decryption operations",
		Flags:   fileFlags,
		Action: func(c *cli.Context) error {
			return cli.ShowCommandHelp(c, "")
		},
		Subcommands: []cli.Command{
			{
				Name: "all",
				Flags: []cli.Flag{
					inputFlag,
					outputFlag,
					updateFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					if inputFilePath != os.Stdin.Name() && updateInPlace {
						outputFilePath = inputFilePath
					}
					buffer, err := s.PerformAction("decrypt")
					utils.SafeWrite(buffer, outputFilePath, err)
					return nil
				},
			},
			{
				Name: "recurse",
				Flags: []cli.Flag{
					dirFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					err := utils.ProcessDir(recurseDir, ".sls", "decrypt", outputFilePath, topLevelElement, pk)
					if err != nil {
						logger.Warnf("decrypt: %s", err)
					}
					return nil
				},
			},
			{
				Name: "path",
				Flags: []cli.Flag{
					inputFlag,
					cli.StringFlag{
						Name:        "path, p",
						Usage:       "YAML path to decrypt",
						Destination: &yamlPath,
					},
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					utils.PathAction(&s, yamlPath, "decrypt")

					return nil
				},
			},
		},
	},
	{
		Name:    "rotate",
		Aliases: []string{"r"},
		Usage:   "decrypt existing files and re-encrypt with a new key",
		Flags: []cli.Flag{
			dirFlag,
			cli.StringFlag{
				Name:        "infile, f",
				Usage:       "input file",
				Destination: &inputFilePath,
			},
		},
		Action: func(c *cli.Context) error {
			setProfile()
			if inputFilePath != "" {
				pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
				s := sls.New(inputFilePath, pk, topLevelElement)
				buf, err := s.PerformAction("rotate")
				utils.SafeWrite(buf, outputFilePath, err)
			} else {
				pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
				err := utils.ProcessDir(recurseDir, ".sls", "rotate", outputFilePath, topLevelElement, pk)
				if err != nil {
					logger.Warnf("rotate: %s", err)
				}
			}
			return nil
		},
	},
	{
		Name:    "keys",
		Aliases: []string{"k"},
		Usage:   "show PGP key IDs used",
		Flags:   fileFlags,
		Action: func(c *cli.Context) error {
			return cli.ShowCommandHelp(c, "")
		},
		Subcommands: []cli.Command{
			{
				Name: "all",
				Flags: []cli.Flag{
					inputFlag,
					outputFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					buffer, err := s.PerformAction("validate")
					if err != nil {
						logger.Fatal(err)
					}
					fmt.Printf("%s\n", buffer.String())
					return nil
				},
			},
			{
				Name: "recurse",
				Flags: []cli.Flag{
					dirFlag,
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					err := utils.ProcessDir(recurseDir, ".sls", "validate", outputFilePath, topLevelElement, pk)
					if err != nil {
						logger.Warnf("keys: %s", err)
					}
					return nil
				},
			},
			{
				Name: "path",
				Flags: []cli.Flag{
					inputFlag,
					cli.StringFlag{
						Name:        "path, p",
						Usage:       "YAML path to examine",
						Destination: &yamlPath,
					},
				},
				Action: func(c *cli.Context) error {
					setProfile()
					pk = pki.New(pgpKeyName, publicKeyRing, secretKeyRing)
					s := sls.New(inputFilePath, pk, topLevelElement)
					utils.PathAction(&s, yamlPath, "validate")

					return nil
				},
			},
		},
	},
}

func main() {
	gpgHome := os.Getenv("GNUPGHOME")
	if gpgHome != "" {
		defaultPubRing = fmt.Sprintf("%s/pubring.gpg", gpgHome)
		defaultSecRing = fmt.Sprintf("%s/secring.gpg", gpgHome)
	}

	var appFlags = []cli.Flag{
		cli.StringFlag{
			Name:        "profile, prof",
			Usage:       "default profile to use in the config file",
			Destination: &defaultProfileName,
		},
		cli.StringFlag{
			Name:        "pubring, pub",
			Value:       defaultPubRing,
			Usage:       "PGP public keyring",
			Destination: &publicKeyRing,
		},
		cli.StringFlag{
			Name:        "secring, sec",
			Value:       defaultSecRing,
			Usage:       "PGP private keyring",
			Destination: &secretKeyRing,
		},
		cli.StringFlag{
			Name:        "pgp_key, k",
			Value:       defaultKeyName,
			Usage:       "PGP key name, email, or ID to use for encryption",
			Destination: &pgpKeyName,
		},
		cli.StringFlag{
			Name:        "element, e",
			Usage:       "Name of the top level element under which encrypted key/value pairs are kept",
			Destination: &topLevelElement,
		},
	}

	app := cli.NewApp()
	app.Version = "1.0.404"
	app.Authors = []cli.Author{
		{
			Name:  "Ed Silva",
			Email: "ed.silva@everbridge.com",
		},
	}

	cli.AppHelpTemplate = appHelp

	app.Copyright = "(c) 2018 Everbridge, Inc."
	app.Usage = "Create and update encrypted content or decrypt encrypted content."
	app.Flags = appFlags

	app.Commands = appCommands

	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal(err)
	}
}

func setProfile() {
	profile := readConfigFile()
	if profile.Name != "" {
		setDefaultsFromProfile(profile)
	}
}

func setDefaultsFromProfile(profile GSPProfile) {
	if profile.DefaultKey != "" {
		pgpKeyName = profile.DefaultKey
	}
	if profile.GnupgHome != "" {
		defaultPubRing = fmt.Sprintf("%s/pubring.gpg", profile.GnupgHome)
		defaultSecRing = fmt.Sprintf("%s/secring.gpg", profile.GnupgHome)
	} else {
		if profile.DefaultPubRing != "" {
			defaultPubRing = profile.DefaultPubRing
		}
		if profile.DefaultSecRing != "" {
			defaultSecRing = profile.DefaultSecRing
		}
	}
}

func createConfigPath() string {
	var usr, _ = user.Current()
	configFile := filepath.Join(usr.HomeDir, ".config/generate-secure-pillar/config.yaml")
	dir := filepath.Dir(configFile)
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		logger.Warnf("error creating config file path: %s", err)
	}

	return configFile
}

func readConfigFile() GSPProfile {
	var profile GSPProfile
	configFile := createConfigPath()
	filename, err := filepath.Abs(configFile)
	if err != nil {
		logger.Fatal(err)
	}

	if _, err = os.Stat(filename); !os.IsNotExist(err) {
		if pgpKeyName == "" {
			var gspConfig = readConfigYaml(filename)
			for _, p := range gspConfig.Profiles {
				if defaultProfileName != "" {
					if p.Name == defaultProfileName {
						profile = p
					}
				} else {
					if p.Default {
						profile = p
					}
				}
			}
		}
	} else {
		// create a default example file
		var buffer bytes.Buffer
		buffer.WriteString("# profiles:\n")
		buffer.WriteString("#   - name: dev:\n")
		buffer.WriteString("#     default: true\n")
		buffer.WriteString("#     default_key: Dev Salt Master\n")
		buffer.WriteString("#     gnupg_home: ~/.gnupg\n")
		buffer.WriteString("#     default_pub_ring: ~/.gnupg/pubring.gpg\n")
		buffer.WriteString("#     default_sec_ring: ~/.gnupg/secring.gpg\n")
		err := ioutil.WriteFile(configFile, buffer.Bytes(), 0644)
		if err != nil {
			logger.Warn("can't write default config file: %s", err)
		}
	}
	return profile
}

func readConfigYaml(filename string) GSPConfig {
	var gspConfig GSPConfig
	yamlData, err := ioutil.ReadFile(filename)
	if err != nil {
		logger.Fatal("error reading config file: ", err)
	}

	err = yaml.Unmarshal(yamlData, &gspConfig)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Unable to parse %s: %s\n", filename, err))
	}
	return gspConfig
}
