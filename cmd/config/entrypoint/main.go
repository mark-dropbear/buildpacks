// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Implements config/entrypoint buildpack.
// The entrypoint buildpack sets the image entrypoint based on an environment variable or Procfile.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/buildpacks/pkg/appyaml"
	"github.com/GoogleCloudPlatform/buildpacks/pkg/env"
	gcp "github.com/GoogleCloudPlatform/buildpacks/pkg/gcpbuildpack"
)

var (
	processRe = regexp.MustCompile(`(?m)^(\w+):\s*(.+)$`)
)

func main() {
	gcp.Main(detectFn, buildFn)
}

func detectFn(ctx *gcp.Context) (gcp.DetectResult, error) {
	if os.Getenv(env.Entrypoint) != "" {
		return gcp.OptInEnvSet(env.Entrypoint), nil
	}
	if ctx.FileExists("Procfile") {
		return gcp.OptInFileFound("Procfile"), nil
	}
	if entrypoint, _ := appyaml.EntrypointIfExists(ctx.ApplicationRoot()); entrypoint != "" {
		ctx.Logf("Using entrypoint from app.yaml.")
		return gcp.OptIn("Found the app.yaml file specified by GAE_APPLICATION_YAML_PATH."), nil
	}
	return gcp.OptOut(fmt.Sprintf(
		"%s not set, no valid entrypoint in app.yaml and Procfile not found", env.Entrypoint)), nil
}

func buildFn(ctx *gcp.Context) error {
	if entrypoint := os.Getenv(env.Entrypoint); entrypoint != "" {
		ctx.AddProcess(gcp.WebProcess, []string{entrypoint}, gcp.AsDefaultProcess())
		ctx.Logf("Using entrypoint from environment variable %s: %s", env.Entrypoint, entrypoint)
		return nil
	}

	if ctx.FileExists("Procfile") {
		b := ctx.ReadFile("Procfile")
		return addProcfileProcesses(ctx, string(b))
	}

	entrypoint, err := appyaml.EntrypointIfExists(ctx.ApplicationRoot())
	if err != nil {
		return gcp.UserErrorf(fmt.Sprintf(
			"app.yaml env var set but the specified app.yaml file doesn't exist."))
	}
	if entrypoint != "" {
		ctx.AddProcess(gcp.WebProcess, []string{entrypoint}, gcp.AsDefaultProcess())
		ctx.Logf("Using entrypoint from app.yaml.")
		return nil
	}

	return gcp.UserErrorf(fmt.Sprintf(
		"%s not set, no valid entrypoint config in Procfile or app.yaml.", env.Entrypoint))
}

// addProcfileProcesses adds all processes from the given Procfile contents.
func addProcfileProcesses(ctx *gcp.Context, content string) error {
	matches := processRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return gcp.UserErrorf("did not find any processes in Procfile")
	}

	found := make(map[string]bool, len(matches))
	for _, match := range matches {
		// Sanity check, if this fails there is a mistake in the regex.
		// One group for overall match and two subgroups.
		if len(match) != 3 {
			return gcp.InternalErrorf("invalid process match, want slice of two strings, got: %v", match)
		}
		name, command := match[1], strings.TrimSpace(match[2])
		if found[name] {
			ctx.Warnf("Skipping duplicate %s process: %s", gcp.WebProcess, command)
			continue
		}
		found[name] = true

		if name == gcp.WebProcess {
			ctx.Logf("Using entrypoint from Procfile: %s", command)
			ctx.AddProcess(name, []string{command}, gcp.AsDefaultProcess())
		} else {
			ctx.AddProcess(name, []string{command})
		}
	}

	if !found[gcp.WebProcess] {
		return gcp.UserErrorf("web process not found in Procfile: %#v", matches)
	}
	return nil
}
