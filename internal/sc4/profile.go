package sc4

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type rendererFiles []string

func (r *rendererFiles) String() string { return strings.Join(*r, ", ") }

func (r *rendererFiles) Set(path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".dll", ".ini":
		*r = append(*r, path)
		return nil
	default:
		return errors.New("renderer file must be a DLL or INI")
	}
}

func packagePaths(inputs []string) ([]string, error) {
	var packages []string
	seen := make(map[string]bool)
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		paths := []string{input}
		if info.IsDir() {
			children, err := os.ReadDir(input)
			if err != nil {
				return nil, err
			}
			paths = nil
			for _, child := range children {
				if !child.IsDir() && strings.EqualFold(filepath.Ext(child.Name()), ".dat") {
					paths = append(paths, filepath.Join(input, child.Name()))
				}
			}
			if len(paths) == 0 {
				return nil, fmt.Errorf("%s contains no DAT packages", input)
			}
		}
		for _, path := range paths {
			if !strings.EqualFold(filepath.Ext(path), ".dat") {
				return nil, fmt.Errorf("%s is not a DAT package", path)
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			absolute, err = filepath.EvalSymlinks(absolute)
			if err != nil {
				return nil, err
			}
			key := strings.ToLower(absolute)
			if !seen[key] {
				seen[key] = true
				packages = append(packages, absolute)
			}
		}
	}
	if len(packages) == 0 {
		return nil, errors.New("select at least one DAT package or package directory")
	}
	return packages, nil
}

func copyRegular(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}

func copyRegions(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a Regions directory", source)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.Mkdir(target, 0755)
		}
		return copyRegular(path, target)
	})
}

func stageProfile(template, destination string, packages, renderer []string, diagnostics io.Writer) error {
	template, err := filepath.Abs(template)
	if err != nil {
		return err
	}
	template, err = filepath.EvalSymlinks(template)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return err
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	if relative, err := filepath.Rel(template, destination); err == nil &&
		relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("stage the new profile outside its template")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", destination)
	}

	names := make(map[string]bool)
	allFiles := append(append([]string(nil), packages...), renderer...)
	for _, path := range allFiles {
		name := strings.ToLower(filepath.Base(path))
		if names[name] {
			return fmt.Errorf("duplicate Windows plugin filename: %s", name)
		}
		names[name] = true
	}
	stage, err := os.MkdirTemp(parent, ".sc4-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyRegions(filepath.Join(template, "Regions"), filepath.Join(stage, "Regions")); err != nil {
		return err
	}
	const configName = "SimCity 4.cfg"
	if err := copyRegular(filepath.Join(template, configName), filepath.Join(stage, configName)); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return err
	}
	plugins := filepath.Join(stage, "Plugins")
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	var copiedPackages []string
	for i, path := range allFiles {
		target := filepath.Join(plugins, filepath.Base(path))
		if err := copyRegular(path, target); err != nil {
			return err
		}
		if i < len(packages) {
			copiedPackages = append(copiedPackages, target)
		}
	}
	if err := verifyPackages(copiedPackages, diagnostics); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", destination)
	}
	return os.Rename(stage, destination)
}

func runStageProfile(args []string, output io.Writer) error {
	var renderer rendererFiles
	flags := flag.NewFlagSet("stage-profile", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Var(&renderer, "renderer", "local renderer DLL or INI to copy (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 3 {
		return errors.New("usage: simcity4-uhd stage-profile [-renderer FILE] TEMPLATE OUTPUT PACKAGE_OR_DIRECTORY ...")
	}
	packages, err := packagePaths(flags.Args()[2:])
	if err != nil {
		return err
	}
	if err := stageProfile(flags.Arg(0), flags.Arg(1), packages, renderer, output); err != nil {
		return err
	}
	fmt.Fprintf(output, "Staged isolated profile: %s\n", flags.Arg(1))
	return nil
}
