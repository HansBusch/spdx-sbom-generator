// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spdx/spdx-sbom-generator/pkg/runner"
	"github.com/spdx/spdx-sbom-generator/pkg/runner/options"
	"github.com/spf13/cobra"
)

const jsonLogFormat = "json"
const defaultLogLevel = "info"

// provided through ldflags on build
var (
	version string
)

var rootCmd = &cobra.Command{
	Use:   "sbomgen",
	Short: "Output Package Manager dependency on SPDX format",
	Long:  "Output Package Manager dependency on SPDX format",
	Run:   generate,
}

func main() {
	if version == "" {
		version = "source-code"
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
func init() {
	rootCmd.Flags().StringP("path", "p", ".", "the path to package file or the path to a directory which will be recursively analyzed for the package files (default '.')")
	rootCmd.Flags().BoolP("include-license-text", "i", false, " Include full license text (default: false)")
	rootCmd.Flags().BoolP("license-concluded", "", false, " Accept declared license as concluded (default: false)")
	rootCmd.Flags().StringP("schema", "s", "2.3", "<version> Target schema version (default: '2.3')")
	rootCmd.Flags().StringP("output-dir", "o", "", "<output> directory to write SPDX doc (default: if not specified, doc is written to stdout)")
	rootCmd.Flags().StringP("format", "f", "spdx", "output file format (default: spdx)")
	rootCmd.Flags().StringP("global-settings", "g", "", "Alternate path for the global settings file for Java Maven (default 'mvn settings.xml')")

	//rootCmd.MarkFlagRequired("path")
	cobra.OnInitialize(setupLogger)
}

func parseOutputFormat(formatOption string) options.OutputFormat {
	switch processedFormatOption := strings.ToLower(formatOption); processedFormatOption {
	case "spdx":
		return options.OutputFormatSpdx
	case "json":
		return options.OutputFormatJson
	default:
		return options.OutputFormatSpdx
	}
}

func setupLogger() {
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	if os.Getenv("LOG_FORMAT") == jsonLogFormat {
		log.SetFormatter(&log.JSONFormatter{})
	}

	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = defaultLogLevel
	}

	logLevel, err := log.ParseLevel(level)
	if err != nil {
		logLevel = log.DebugLevel
	}

	log.SetLevel(logLevel)
}

func generate(cmd *cobra.Command, args []string) {
	log.Info("Starting to generate SPDX ...")
	checkOpt := func(opt string) string {
		cmdOpt, err := cmd.Flags().GetString(opt)
		if err != nil {
			log.Fatalf("Failed to read command option %v", err)
		}

		return cmdOpt
	}
	path := checkOpt("path")
	outputDir := checkOpt("output-dir")
	schema := checkOpt("schema")
	format := parseOutputFormat(checkOpt("format"))
	license, err := cmd.Flags().GetBool("include-license-text")
	if err != nil {
		log.Fatalf("Failed to read command option: %v", err)
	}
	concluded, _ := cmd.Flags().GetBool("license-concluded")
	globalSettingFile := checkOpt("global-settings")
	fmt.Print("concluded = " + strconv.FormatBool(concluded))

	opts := options.Options{
		SchemaVersion:     schema,
		Indent:            4,
		Version:           version,
		License:           license,
		Depth:             "",
		Slug:              "",
		OutputDir:         outputDir,
		Format:            format,
		GlobalSettingFile: globalSettingFile,
		Path:              path,
		Plugins:           options.DefaultPlugins,
		LicenseConcluded:  concluded,
	}

	err = runner.NewWithOptions(opts).CreateSBOM()

	if err != nil {
		log.Fatalf("error creating SBOM, err: %s", err.Error())
	}

}
