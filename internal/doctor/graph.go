package doctor

import (
	"fmt"
	"io"
	"strconv"

	"github.com/baldaworks/callee/internal/registry"
)

// WriteGraph writes the complete static registry reference graph.
func WriteGraph(output io.Writer, configured *registry.AgentRegistry, format string) error {
	switch format {
	case "text":
		return writeTextGraph(output, configured)
	case "mermaid":
		return writeMermaidGraph(output, configured)
	case "dot":
		return writeDOTGraph(output, configured)
	default:
		return fmt.Errorf("unsupported graph format %q (want text, mermaid, or dot)", format)
	}
}

func writeTextGraph(output io.Writer, configured *registry.AgentRegistry) error {
	for _, item := range configured.Agents() {
		if _, err := fmt.Fprintf(output, "%s [%s]\n", item.ID, item.Kind); err != nil {
			return err
		}

		for _, child := range item.Spec.Children {
			annotation := fmt.Sprintf(" canEscalate=%t", child.CanEscalate)
			if child.Alias != "" {
				annotation = " alias=" + child.Alias + annotation
			}

			if _, err := fmt.Fprintf(output, "  -> %s%s\n", child.Ref, annotation); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeMermaidGraph(output io.Writer, configured *registry.AgentRegistry) error {
	if _, err := fmt.Fprintln(output, "flowchart TD"); err != nil {
		return err
	}

	ids := configured.IDs()
	nodes := make(map[string]string, len(ids))

	for index, id := range ids {
		nodes[id] = fmt.Sprintf("n%d", index)

		item, _ := configured.GetAgent(id)
		if _, err := fmt.Fprintf(output, "  %s[%s]\n", nodes[id], strconv.Quote(id+" ["+string(item.Kind)+"]")); err != nil {
			return err
		}
	}

	for _, item := range configured.Agents() {
		for _, child := range item.Spec.Children {
			label := child.Alias
			if label == "" {
				label = "ref"
			}

			label += fmt.Sprintf(", canEscalate=%t", child.CanEscalate)

			if _, err := fmt.Fprintf(output, "  %s -->|%s| %s\n", nodes[item.ID], label, nodes[child.Ref]); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeDOTGraph(output io.Writer, configured *registry.AgentRegistry) error {
	if _, err := fmt.Fprintln(output, "digraph callee {"); err != nil {
		return err
	}

	for _, item := range configured.Agents() {
		if _, err := fmt.Fprintf(output, "  %s [label=%s];\n", strconv.Quote(item.ID), strconv.Quote(item.ID+" ["+string(item.Kind)+"]")); err != nil {
			return err
		}

		for _, child := range item.Spec.Children {
			label := child.Alias
			if label == "" {
				label = "ref"
			}

			label += fmt.Sprintf(", canEscalate=%t", child.CanEscalate)

			if _, err := fmt.Fprintf(output, "  %s -> %s [label=%s];\n", strconv.Quote(item.ID), strconv.Quote(child.Ref), strconv.Quote(label)); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(output, "}")

	return err
}
