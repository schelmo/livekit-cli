// Copyright 2023 LiveKit, Inc.
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

package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v3"

	"github.com/livekit/livekit-cli/pkg/config"
)

var (
	ProjectCommands = []*cli.Command{
		{
			Name:     "project",
			Usage:    "Add or remove projects and view existing project properties",
			Category: "Core",
			Before:   loadProjectConfig,
			Commands: []*cli.Command{
				{
					Name:      "add",
					Usage:     "Add a new project",
					UsageText: "lk project add PROJECT_NAME",
					ArgsUsage: "PROJECT_NAME",
					Action:    addProject,
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:  "url",
							Usage: "`URL` of the LiveKit server",
						},
						&cli.StringFlag{
							Name:  "api-key",
							Usage: "Project `KEY`",
						},
						&cli.StringFlag{
							Name:  "api-secret",
							Usage: "Project `SECRET`",
						},
					},
				},
				{
					Name:      "list",
					Usage:     "List all configured projects",
					UsageText: "lk project list",
					Action:    listProjects,
				},
				{
					Name:      "remove",
					Usage:     "Remove an existing project from config",
					UsageText: "lk project remove PROJECT_NAME",
					ArgsUsage: "PROJECT_NAME",
					Action:    removeProject,
				},
				{
					Name:      "set-default",
					Usage:     "Set a project as default to use with other commands",
					UsageText: "lk project set-default PROJECT_NAME",
					ArgsUsage: "PROJECT_NAME",
					Action:    setDefaultProject,
				},
			},
		},
	}

	cliConfig      *config.CLIConfig
	defaultProject *config.ProjectConfig
	nameRegex      = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
)

func loadProjectConfig(ctx context.Context, cmd *cli.Command) error {
	conf, err := config.LoadOrCreate()
	if err != nil {
		return err
	}
	cliConfig = conf

	if cliConfig.DefaultProject != "" {
		for _, p := range cliConfig.Projects {
			if p.Name == cliConfig.DefaultProject {
				defaultProject = &p
				break
			}
		}
	}
	return nil
}

func addProject(ctx context.Context, cmd *cli.Command) error {
	p := config.ProjectConfig{}
	var prompt promptui.Prompt

	// URL
	var err error
	validateURL := func(val string) error {
		if !strings.HasPrefix(val, "http") && !strings.HasPrefix(val, "ws") {
			return errors.New("URL must start with http(s) or ws(s)")
		}
		_, err := url.Parse(val)
		return err
	}
	if p.URL = cmd.String("url"); p.URL != "" {
		if err = validateURL(p.URL); err != nil {
			return err
		}
		fmt.Println("URL:", p.URL)
	} else {
		prompt = promptui.Prompt{
			Label:    "URL",
			Validate: validateURL,
		}
		if p.URL, err = prompt.Run(); err != nil {
			return err
		}
	}

	// API key
	validateKey := func(val string) error {
		if len(val) < 3 {
			return errors.New("API key must be at least 3 characters")
		}
		return nil
	}
	if p.APIKey = cmd.String("api-key"); p.APIKey != "" {
		if err = validateKey(p.APIKey); err != nil {
			return err
		}
		fmt.Println("API Key:", p.APIKey)
	} else {
		prompt = promptui.Prompt{
			Label:    "API Key",
			Validate: validateKey,
		}
		if p.APIKey, err = prompt.Run(); err != nil {
			return err
		}
	}

	// API Secret
	if p.APISecret = cmd.String("api-secret"); p.APISecret != "" {
		if err = validateKey(p.APISecret); err != nil {
			return err
		}
		fmt.Println("API Secret:", p.APISecret)
	} else {
		prompt = promptui.Prompt{
			Label:    "API Secret",
			Validate: validateKey,
		}
		if p.APISecret, err = prompt.Run(); err != nil {
			return err
		}
	}

	// Name
	validateName := func(val string) error {
		if !nameRegex.MatchString(val) {
			return errors.New("name can only contain alphanumeric characters, dashes and underscores")
		}
		// cannot conflict with existing projects
		for _, p := range cliConfig.Projects {
			if p.Name == val {
				return errors.New("name already exists")
			}
		}
		return nil
	}

	if p.Name = cmd.Args().Get(0); p.Name != "" {
		if err = validateName(p.Name); err != nil {
			return err
		}
	} else {
		prompt = promptui.Prompt{
			Label:    "Give it a name for later reference",
			Validate: validateName,
		}
		if p.Name, err = prompt.Run(); err != nil {
			return err
		}
	}

	// if it's first project, make it default
	if defaultProject != nil {
		prompt = promptui.Prompt{
			Label:     "Make this project default?",
			IsConfirm: true,
		}
		if _, err = prompt.Run(); err != nil && err != promptui.ErrAbort {
			return err
		}
		if err == nil {
			cliConfig.DefaultProject = p.Name
		}
	} else {
		cliConfig.DefaultProject = p.Name
	}
	cliConfig.Projects = append(cliConfig.Projects, p)

	// save config
	if err = cliConfig.PersistIfNeeded(); err != nil {
		return err
	}

	fmt.Println("Added project", p.Name)

	return nil
}

func listProjects(ctx context.Context, cmd *cli.Command) error {
	if len(cliConfig.Projects) == 0 {
		fmt.Println("No projects configured, use `lk project add` to add a new project.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeader([]string{"Name", "URL", "API Key", "Default"})
	for _, p := range cliConfig.Projects {
		table.Append([]string{p.Name, p.URL, p.APIKey, fmt.Sprint(p.Name == cliConfig.DefaultProject)})
	}
	table.Render()
	return nil
}

func removeProject(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		_ = cli.ShowSubcommandHelp(cmd)
		return errors.New("project name is required")
	}
	name := cmd.Args().First()

	var newProjects []config.ProjectConfig
	for _, p := range cliConfig.Projects {
		if p.Name == name {
			continue
		}
		newProjects = append(newProjects, p)
	}
	cliConfig.Projects = newProjects

	if cliConfig.DefaultProject == name {
		cliConfig.DefaultProject = ""
	}

	if err := cliConfig.PersistIfNeeded(); err != nil {
		return err
	}

	fmt.Println("Removed project", name)

	return nil
}

func setDefaultProject(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		_ = cli.ShowSubcommandHelp(cmd)
		return errors.New("project name is required")
	}
	name := cmd.Args().First()

	for _, p := range cliConfig.Projects {
		if p.Name == name {
			cliConfig.DefaultProject = name
			if err := cliConfig.PersistIfNeeded(); err != nil {
				return err
			}
			fmt.Println("Default project set to", name)
			return nil
		}
	}

	return errors.New("project not found")
}
