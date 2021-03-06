// Copyright 2016-2018, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workspace

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/resource/config"
	"github.com/pulumi/pulumi/pkg/tokens"
	"github.com/pulumi/pulumi/pkg/util/contract"
)

const (
	defaultProjectName = "project"

	// This file will be ignored when copying from the template cache to
	// a project directory.
	pulumiTemplateManifestFile = ".pulumi.template.yaml"
)

// Template represents a project template.
type Template struct {
	// The name of the template.
	Name string `json:"name" yaml:"name"`
	// Optional description of the template, also used as a default project description.
	Description string `json:"description" yaml:"description"`
	// Optional bool which determines whether dependencies should be installed after project creation.
	InstallDependencies bool `json:"installdependencies" yaml:"installdependencies"`
	// Optional default config values.
	Config map[config.Key]string `json:"config" yaml:"config"`
}

// LoadLocalTemplate returns a local template.
func LoadLocalTemplate(name string) (Template, error) {
	templateDir, err := GetTemplateDir(name)
	if err != nil {
		return Template{}, err
	}

	info, err := os.Stat(templateDir)
	if err != nil {
		return Template{}, err
	}
	if !info.IsDir() {
		return Template{}, errors.Errorf("template '%s' in %s is not a directory", name, templateDir)
	}

	// Read the description from the manifest (if it exists).
	template, err := readTemplateManifest(filepath.Join(templateDir, pulumiTemplateManifestFile))
	if err != nil && !os.IsNotExist(err) {
		return Template{}, err
	}

	template.Name = name
	return template, nil
}

// ListLocalTemplates returns a list of local templates.
func ListLocalTemplates() ([]Template, error) {
	templateDir, err := GetTemplateDir("")
	if err != nil {
		return nil, err
	}

	infos, err := ioutil.ReadDir(templateDir)
	if err != nil {
		return nil, err
	}

	var templates []Template
	for _, info := range infos {
		if info.IsDir() {
			template, err := LoadLocalTemplate(info.Name())
			if err != nil {
				return nil, err
			}
			templates = append(templates, template)
		}
	}
	return templates, nil
}

// InstallTemplate installs a template tarball into the local cache.
func InstallTemplate(name string, tarball io.ReadCloser) error {
	contract.Require(name != "", "name")
	contract.Require(tarball != nil, "tarball")

	var templateDir string
	var err error

	// Get the template directory.
	if templateDir, err = GetTemplateDir(name); err != nil {
		return err
	}

	// Delete the directory if it exists.
	if err = os.RemoveAll(templateDir); err != nil {
		return errors.Wrapf(err, "removing existing template directory %s", templateDir)
	}

	// Ensure it exists since we may have just deleted it.
	if err = os.MkdirAll(templateDir, 0700); err != nil {
		return errors.Wrapf(err, "creating template directory %s", templateDir)
	}

	// Extract the tarball to its directory.
	if err = extractTarball(tarball, templateDir); err != nil {
		return errors.Wrapf(err, "extracting template to %s", templateDir)
	}

	// On Windows, we need to replace \n with \r\n. We'll just do this as a separate step.
	if runtime.GOOS == "windows" {
		if err = fixWindowsLineEndings(templateDir); err != nil {
			return errors.Wrapf(err, "fixing line endings in %s", templateDir)
		}
	}

	return nil
}

// CopyTemplateFilesDryRun does a dry run of copying a template to a destination directory,
// to ensure it won't overwrite any files.
func (template Template) CopyTemplateFilesDryRun(destDir string) error {
	var err error
	var sourceDir string
	if sourceDir, err = GetTemplateDir(template.Name); err != nil {
		return err
	}

	var existing []string
	err = walkFiles(sourceDir, destDir, func(info os.FileInfo, source string, dest string) error {
		if destInfo, statErr := os.Stat(dest); statErr == nil && !destInfo.IsDir() {
			existing = append(existing, filepath.Base(dest))
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(existing) > 0 {
		return newExistingFilesError(existing)
	}
	return nil
}

// CopyTemplateFiles does the actual copy operation to a destination directory.
func (template Template) CopyTemplateFiles(
	destDir string, force bool, projectName string, projectDescription string) error {

	sourceDir, err := GetTemplateDir(template.Name)
	if err != nil {
		return err
	}

	return walkFiles(sourceDir, destDir, func(info os.FileInfo, source string, dest string) error {
		if info.IsDir() {
			// Create the destination directory.
			return os.Mkdir(dest, 0700)
		}

		// Read the source file.
		b, err := ioutil.ReadFile(source)
		if err != nil {
			return err
		}

		// Transform only if it isn't a binary file.
		result := b
		if !isBinary(b) {
			transformed := transform(string(b), projectName, projectDescription)
			result = []byte(transformed)
		}

		// Write to the destination file.
		err = writeAllBytes(dest, result, force)
		if err != nil {
			// An existing file has shown up in between the dry run and the actual copy operation.
			if os.IsExist(err) {
				return newExistingFilesError([]string{filepath.Base(dest)})
			}
		}
		return err
	})
}

// GetTemplateDir returns the directory in which templates on the current machine are stored.
func GetTemplateDir(name string) (string, error) {
	u, err := user.Current()
	if u == nil || err != nil {
		return "", errors.Wrap(err, "getting user home directory")
	}
	dir := filepath.Join(u.HomeDir, BookkeepingDir, TemplateDir)
	if name != "" {
		dir = filepath.Join(dir, name)
	}
	return dir, nil
}

// IsValidProjectName returns true if the project name is a valid name.
func IsValidProjectName(name string) bool {
	return tokens.IsPackageName(name)
}

// ValueOrSanitizedDefaultProjectName returns the value or a sanitized valid project name
// based on defaultNameToSanitize.
func ValueOrSanitizedDefaultProjectName(name string, defaultNameToSanitize string) string {
	if name != "" {
		return name
	}
	return getValidProjectName(defaultNameToSanitize)
}

// ValueOrDefaultProjectDescription returns the value or defaultDescription.
func ValueOrDefaultProjectDescription(description string, defaultDescription string) string {
	if description != "" {
		return description
	}
	if defaultDescription != "" {
		return defaultDescription
	}
	return ""
}

// getValidProjectName returns a valid project name based on the passed-in name.
func getValidProjectName(name string) string {
	// If the name is valid, return it.
	if IsValidProjectName(name) {
		return name
	}

	// Otherwise, try building-up the name, removing any invalid chars.
	var result string
	for i := 0; i < len(name); i++ {
		temp := result + string(name[i])
		if IsValidProjectName(temp) {
			result = temp
		}
	}

	// If we couldn't come up with a valid project name, fallback to a default.
	if result == "" {
		result = defaultProjectName
	}

	return result
}

// extractTarball extracts the tarball to the specified destination directory.
func extractTarball(tarball io.ReadCloser, destDir string) error {
	// Unzip and untar the file as we go.
	defer contract.IgnoreClose(tarball)
	gzr, err := gzip.NewReader(tarball)
	if err != nil {
		return errors.Wrapf(err, "unzipping")
	}
	r := tar.NewReader(gzr)
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "untarring")
		}

		path := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create any directories as needed.
			if _, err := os.Stat(path); err != nil {
				if err = os.MkdirAll(path, 0700); err != nil {
					return errors.Wrapf(err, "untarring dir %s", path)
				}
			}
		case tar.TypeReg:
			// Expand files into the target directory.
			dst, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return errors.Wrapf(err, "opening file %s for untar", path)
			}
			defer contract.IgnoreClose(dst)
			if _, err = io.Copy(dst, r); err != nil {
				return errors.Wrapf(err, "untarring file %s", path)
			}
		default:
			return errors.Errorf("unexpected plugin file type %s (%v)", header.Name, header.Typeflag)
		}
	}
	return nil
}

// walkFiles is a helper that walks the directories/files in a source directory
// and performs an action for each item.
func walkFiles(sourceDir string, destDir string,
	actionFn func(info os.FileInfo, source string, dest string) error) error {

	contract.Require(sourceDir != "", "sourceDir")
	contract.Require(destDir != "", "destDir")
	contract.Require(actionFn != nil, "actionFn")

	infos, err := ioutil.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		name := info.Name()
		source := filepath.Join(sourceDir, name)
		dest := filepath.Join(destDir, name)

		if info.IsDir() {
			if err := actionFn(info, source, dest); err != nil {
				return err
			}

			if err := walkFiles(source, dest, actionFn); err != nil {
				return err
			}
		} else {
			// Ignore template manifest file.
			if name == pulumiTemplateManifestFile {
				continue
			}

			if err := actionFn(info, source, dest); err != nil {
				return err
			}
		}
	}

	return nil
}

// readTemplateManifest reads a template manifest file.
func readTemplateManifest(filename string) (Template, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return Template{}, err
	}

	var manifest Template
	err = yaml.Unmarshal(b, &manifest)
	if err != nil {
		return Template{}, err
	}

	return manifest, nil
}

// newExistingFilesError returns a new error from a list of existing file names
// that would be overwritten.
func newExistingFilesError(existing []string) error {
	contract.Assert(len(existing) > 0)
	message := "creating this template will make changes to existing files:\n"
	for _, file := range existing {
		message = message + fmt.Sprintf("  overwrite   %s\n", file)
	}
	message = message + "\nrerun the command and pass --force to accept and create"
	return errors.New(message)
}

// transform returns a new string with ${PROJECT} and ${DESCRIPTION} replaced by
// the value of projectName and projectDescription.
func transform(content string, projectName string, projectDescription string) string {
	content = strings.Replace(content, "${PROJECT}", projectName, -1)
	content = strings.Replace(content, "${DESCRIPTION}", projectDescription, -1)
	return content
}

// writeAllBytes writes the bytes to the specified file, with an option to overwrite.
func writeAllBytes(filename string, bytes []byte, overwrite bool) error {
	flag := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flag = flag | os.O_TRUNC
	} else {
		flag = flag | os.O_EXCL
	}

	f, err := os.OpenFile(filename, flag, 0600)
	if err != nil {
		return err
	}
	defer contract.IgnoreClose(f)

	_, err = f.Write(bytes)
	return err
}

// isBinary returns true if a zero byte occurs within the first
// 8000 bytes (or the entire length if shorter). This is the
// same approach that git uses to determine if a file is binary.
func isBinary(bytes []byte) bool {
	const firstFewBytes = 8000

	length := len(bytes)
	if firstFewBytes < length {
		length = firstFewBytes
	}

	for i := 0; i < length; i++ {
		if bytes[i] == 0 {
			return true
		}
	}

	return false
}

// fixWindowsLineEndings will go through the sourceDir, read each file, replace \n with \r\n,
// and save the changes.
// It'd be more efficient to do this during tarball extraction, but this is sufficient for now.
func fixWindowsLineEndings(sourceDir string) error {
	return walkFiles(sourceDir, sourceDir, func(info os.FileInfo, source string, dest string) error {
		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// Read the source file.
		b, err := ioutil.ReadFile(source)
		if err != nil {
			return err
		}

		// Transform only if it isn't a binary file.
		result := b
		if !isBinary(b) {
			content := string(b)
			content = strings.Replace(content, "\n", "\r\n", -1)
			result = []byte(content)
		}

		// Write to the destination file.
		err = writeAllBytes(dest, result, true /*overwrite*/)
		if err != nil {
			// An existing file has shown up in between the dry run and the actual copy operation.
			if os.IsExist(err) {
				return newExistingFilesError([]string{filepath.Base(dest)})
			}
		}
		return err
	})
}
