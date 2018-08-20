# generate-secure-pillar

[![Average time to resolve an issue](http://isitmaintained.com/badge/resolution/Everbridge/generate-secure-pillar.svg)](https://isitmaintained.com/project/Everbridge/generate-secure-pillar "Average time to resolve an issue")
[![Percentage of issues still open](http://isitmaintained.com/badge/open/Everbridge/generate-secure-pillar.svg)](https://isitmaintained.com/project/Everbridge/generate-secure-pillar "Percentage of issues still open")

## Create and update encrypted content or decrypt encrypted content in YAML files

## USAGE

   generate-secure-pillar [global options] command [command options] [arguments...]

## VERSION 1.0.404

## AUTHOR

   Ed Silva <ed.silva@everbridge.com>

## HOMEBREW INSTALL

``` shell
brew tap esilva-everbridge/homebrew-generate-secure-pillar
brew install generate-secure-pillar
```

## Config File Usage

A config file can be used to set default values, an example file is created if there isn't one already with commented out values:

``` shell
# default_key: Salt Master
# gnupg_home: ~/.gnupg
# default_pub_ring: ~/.gnupg/pubring.gpg
# default_sec_ring: ~/.gnupg/secring.gpg
```

## ABOUT PGP KEYS

The PGP keys you import for use with this tool need to be 'trusted' keys.
An easy way to do this is, after importing a key, run the following commands:

``` shell
expect -c "spawn gpg --edit-key '<the PGP key id here>' trust quit; send \"5\ry\r\"; expect eof"
```

(found here: <https://gist.github.com/chrisroos/1205934#gistcomment-2203760)>

## COMMANDS

     create, c   create a new sls file
     update, u   update the value of the given key in the given file
     encrypt, e  perform encryption operations
     decrypt, d  perform decryption operations
     rotate, r   decrypt existing files and re-encrypt with a new key
     keys, k     show PGP key IDs used
     help, h     Shows a list of commands or help for one command

## GLOBAL OPTIONS

- --pubring value, --pub value  PGP public keyring (default: "~/.gnupg/pubring.gpg" or "$GNUPGHOME/pubring.gpg")
- --secring value, --sec value  PGP private keyring (default: "~/.gnupg/secring.gpg" or "$GNUPGHOME/secring.gpg")
- --pgp_key value, -k value     PGP key name, email, or ID to use for encryption
- --debug                       adds line number info to log output
- --element value, -e value     Name of the top level element under which encrypted key/value pairs are kept
- --help, -h                    show help
- --version, -v                 print the version

## COPYRIGHT

   (c) 2018 Everbridge, Inc.

**CAVEAT: YAML files with include statements are not handled properly, so we skip them.**

## EXAMPLES

### create a new sls file

```$ generate-secure-pillar -k "Salt Master" create --name secret_name1 --value secret_value1 --name secret_name2 --value secret_value2 --outfile new.sls```

### add to the new file

```$ generate-secure-pillar -k "Salt Master" update --name new_secret_name --value new_secret_value --file new.sls```

### update an existing value

```$ generate-secure-pillar -k "Salt Master" update --name secret_name --value secret_value3 --file new.sls```

### encrypt all plain text values in a file

```$ generate-secure-pillar -k "Salt Master" encrypt all --file us1.sls --outfile us1.sls```

### or use --update flag

```$ generate-secure-pillar -k "Salt Master" encrypt all --file us1.sls --update```

### encrypt all plain text values in a file under the element 'secret_stuff'

```$ generate-secure-pillar -k "Salt Master" --element secret_stuff encrypt all --file us1.sls --outfile us1.sls```

### recurse through all sls files, encrypting all values

```$ generate-secure-pillar -k "Salt Master" encrypt recurse -d /path/to/pillar/secure/stuff```

### recurse through all sls files, decrypting all values (requires imported private key)

```$ generate-secure-pillar decrypt recurse -d /path/to/pillar/secure/stuff```

### decrypt a specific existing value (requires imported private key)

```$ generate-secure-pillar decrypt path --path "some:yaml:path" --file new.sls```

### decrypt all files and re-encrypt with given key (requires imported private key)

```$ generate-secure-pillar -k "New Salt Master Key" rotate -d /path/to/pillar/secure/stuff```

### show all PGP key IDs used in a file

```$ generate-secure-pillar keys all --file us1.sls```

### show all keys used in all files in a given directory

```$ generate-secure-pillar keys recurse -d /path/to/pillar/secure/stuff```

### show the PGP key ID used for an element at a path in a file

```$ generate-secure-pillar keys path --path "some:yaml:path" --file new.sls```
