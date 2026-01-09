// Copyright 2025 The Kubernetes Authors.
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

package generate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/kubernetes-sigs/kro/api/v1alpha1"
)

var (
	outputFile  string
	openBrowser bool
)

func init() {
	generateDiagramCmd.Flags().StringVar(&outputFile, "output", "", "Output file path (optional)")
	generateDiagramCmd.Flags().BoolVar(&openBrowser, "open", false, "Open the generated diagram in the browser")
}

var generateDiagramCmd = &cobra.Command{
	Use:   "diagram",
	Short: "Generate a diagram from a ResourceGraphDefinition",
	Long: "Generate a diagram from a ResourceGraphDefinition file. This command reads the " +
		"ResourceGraphDefinition and outputs the corresponding diagram",
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.resourceGraphDefinitionFile == "" {
			return fmt.Errorf("ResourceGraphDefinition file is required")
		}

		data, err := os.ReadFile(config.resourceGraphDefinitionFile)
		if err != nil {
			return fmt.Errorf("failed to read ResourceGraphDefinition file: %w", err)
		}

		var rgd v1alpha1.ResourceGraphDefinition
		if err = yaml.Unmarshal(data, &rgd); err != nil {
			return fmt.Errorf("failed to unmarshal ResourceGraphDefinition: %w", err)
		}

		// Handle output file logic
		finalOutput := outputFile
		if openBrowser && finalOutput == "" {
			// Create a temp file if opening in browser but no output file specified
			f, err := os.CreateTemp("", "kro-diagram-*.html")
			if err != nil {
				return fmt.Errorf("failed to create temp file: %w", err)
			}
			finalOutput = f.Name()
			f.Close() // Close it, generateDiagram will write to it
		}

		if err = generateDiagram(&rgd, finalOutput); err != nil {
			return fmt.Errorf("failed to generate diagram: %w", err)
		}

		if openBrowser {
			// Ensure we have an absolute path for the browser
			absPath, err := filepath.Abs(finalOutput)
			if err != nil {
				return fmt.Errorf("failed to resolve absolute path: %w", err)
			}
			// Prepend protocol for browser
			url := "file://" + absPath
			fmt.Printf("Opening diagram: %s\n", url)
			if err := openInBrowser(url); err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}
		} else if finalOutput != "" {
			fmt.Printf("Diagram generated at: %s\n", finalOutput)
		}

		return nil
	},
}

func generateDiagram(rgd *v1alpha1.ResourceGraphDefinition, outputFile string) error {
	rgdGraph, err := createGraphBuilder(rgd)
	if err != nil {
		return fmt.Errorf("failed to setup rgd graph: %w", err)
	}

	graph := charts.NewGraph()

	// Graph layout
	graph.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Width:  "70%",
			Height: "90vh",
		}),
		charts.WithTitleOpts(opts.Title{
			Title:    fmt.Sprintf("Resource Graph: %s", rgd.Name),
			Subtitle: fmt.Sprintf("Topological Order: %v", rgdGraph.TopologicalOrder),
			TitleStyle: &opts.TextStyle{
				FontSize:   18,
				FontWeight: "bold",
			},
			SubtitleStyle: &opts.TextStyle{
				FontSize: 14,
			},
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: opts.Bool(false),
		}),
	)

	resourceOrder := make(map[string]int)
	for i, resourceName := range rgdGraph.TopologicalOrder {
		resourceOrder[resourceName] = i + 1
	}

	// Graph nodes
	nodes := make([]opts.GraphNode, 0, len(rgdGraph.Resources))
	for resourceName := range rgdGraph.Resources {
		order := resourceOrder[resourceName]

		node := opts.GraphNode{
			Name:       fmt.Sprintf("%s\n(%d)", resourceName, order),
			SymbolSize: 40.0,
			ItemStyle: &opts.ItemStyle{
				Color:       "#269103",
				BorderColor: "#333",
				BorderWidth: 1,
			},
		}
		nodes = append(nodes, node)
	}

	// Graph links
	links := make([]opts.GraphLink, 0)
	for resourceName, resource := range rgdGraph.Resources {
		targetOrder := resourceOrder[resourceName]

		for _, dependency := range resource.GetDependencies() {
			sourceOrder := resourceOrder[dependency]

			link := opts.GraphLink{
				Source: fmt.Sprintf("%s\n(%d)", dependency, sourceOrder),
				Target: fmt.Sprintf("%s\n(%d)", resourceName, targetOrder),
				LineStyle: &opts.LineStyle{
					Color: "#000",
				},
			}
			links = append(links, link)
		}
	}

	graph.AddSeries("resource", nodes, links).SetSeriesOptions(
		charts.WithGraphChartOpts(
			opts.GraphChart{
				Force: &opts.GraphForce{
					Repulsion: 5000,
				},
				Roam:           opts.Bool(true),
				EdgeSymbol:     "arrow",
				EdgeSymbolSize: []int{0, 7},
			},
		),
	)

	var w io.Writer = os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	return graph.Render(w)
}

func openInBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
