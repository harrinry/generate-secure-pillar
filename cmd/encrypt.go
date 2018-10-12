// Copyright © 2018 Everbridge, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"os"
	"path/filepath"

	"github.com/Everbridge/generate-secure-pillar/sls"
	"github.com/Everbridge/generate-secure-pillar/utils"
	"github.com/spf13/cobra"
)

// encryptCmd represents the encrypt command
var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "perform encryption operations",
	Run: func(cmd *cobra.Command, args []string) {
		pk := getPki()
		outputFilePath, err := filepath.Abs(cmd.Flag("outfile").Value.String())
		if err != nil {
			logger.Fatal(err)
		}
		inputFilePath, err := filepath.Abs(cmd.Flag("file").Value.String())
		if err != nil {
			logger.Fatal(err)
		}

		// process args
		switch args[0] {
		case all:
			s := sls.New(inputFilePath, pk, topLevelElement)
			if inputFilePath != os.Stdin.Name() && updateInPlace {
				outputFilePath = inputFilePath
			}
			buffer, err := s.PerformAction("encrypt")
			utils.SafeWrite(buffer, outputFilePath, err)
		case recurse:
			recurseDir = cmd.Flag("dir").Value.String()
			err := utils.ProcessDir(recurseDir, ".sls", "encrypt", outputFilePath, topLevelElement, pk)
			if err != nil {
				logger.Warnf("encrypt: %s", err)
			}
		case path:
			yamlPath = cmd.Flag("path").Value.String()
			s := sls.New(inputFilePath, pk, topLevelElement)
			utils.PathAction(&s, yamlPath, "encrypt")
		}
	},
}

func init() {
	rootCmd.AddCommand(encryptCmd)
	encryptCmd.PersistentFlags().StringP("path", "p", "", "YAML path to encrypt")
	encryptCmd.PersistentFlags().StringP("dir", "d", "", "recurse over all .sls files in the given directory")
	encryptCmd.PersistentFlags().StringP("file", "f", os.Stdin.Name(), "input file (defaults to STDIN)")
	encryptCmd.PersistentFlags().StringP("outfile", "o", os.Stdout.Name(), "output file (defaults to STDOUT)")
	encryptCmd.PersistentFlags().BoolP("update", "u", false, "update the input file")
}