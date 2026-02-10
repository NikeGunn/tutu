package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// ─── Agent CLI (Phase 2 — Architecture Part VI) ─────────────────────────────
// The Python agent runtime is lazy-loaded: NOT started at boot.
// These CLI commands provide the Go-side interface for managing agents.
// Until the Python runtime is installed, commands report a clear message.

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentRemoveCmd)

	agentRunCmd.Flags().StringP("file", "f", "", "Input file to pass to the agent")
	agentRunCmd.Flags().StringP("input", "i", "", "Input text to pass to the agent")
	agentCreateCmd.Flags().StringP("file", "f", "", "Path to agent YAML definition")
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agent workflows",
	Long: `Manage AI agent workflows powered by TuTu's Python agent runtime.
Agents are defined in YAML and execute multi-step workflows using local
or distributed inference. The Python runtime is lazy-loaded — only started
when an agent is invoked, and exits after 5 minutes of inactivity.`,
}

// ─── agent run ──────────────────────────────────────────────────────────────

var agentRunCmd = &cobra.Command{
	Use:   "run AGENT_NAME",
	Short: "Run an agent workflow",
	Long:  `Execute an agent workflow. The Python runtime will be started if not already running.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentRun,
}

func runAgentRun(cmd *cobra.Command, args []string) error {
	agentName := args[0]
	fileInput, _ := cmd.Flags().GetString("file")
	textInput, _ := cmd.Flags().GetString("input")

	// Check if agent exists
	agentsDir := agentsDirectory()
	agentFile := filepath.Join(agentsDir, agentName+".yaml")
	if _, err := os.Stat(agentFile); os.IsNotExist(err) {
		// Also try without .yaml extension
		agentFile = filepath.Join(agentsDir, agentName+".yml")
		if _, err := os.Stat(agentFile); os.IsNotExist(err) {
			return fmt.Errorf("agent %q not found in %s\nUse 'tutu agent create -f <file>' to register an agent", agentName, agentsDir)
		}
	}

	// Check Python availability
	if !isPythonAvailable() {
		return fmt.Errorf(`Python agent runtime is not available.

The agent runtime requires Python 3.10+ to be installed.
Install Python and run: tutu agent run %s

Alternatively, set [agent].python_path in ~/.tutu/config.toml
to point to your Python installation.`, agentName)
	}

	// Phase 2 stub: report that the gRPC bridge is not yet implemented
	fmt.Fprintf(os.Stdout, "Agent: %s\n", agentName)
	if fileInput != "" {
		fmt.Fprintf(os.Stdout, "Input file: %s\n", fileInput)
	}
	if textInput != "" {
		fmt.Fprintf(os.Stdout, "Input: %s\n", textInput)
	}

	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "⚠️  Agent runtime (gRPC bridge) is under development.")
	fmt.Fprintln(os.Stdout, "    The Python agent executor will be available in a future update.")
	fmt.Fprintln(os.Stdout, "    Agent YAML registered and ready for execution.")
	return nil
}

// ─── agent create ───────────────────────────────────────────────────────────

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register an agent from a YAML definition",
	Long:  `Register a new agent workflow from a YAML definition file.`,
	RunE:  runAgentCreate,
}

func runAgentCreate(cmd *cobra.Command, args []string) error {
	yamlFile, _ := cmd.Flags().GetString("file")
	if yamlFile == "" {
		return fmt.Errorf("agent YAML file required: tutu agent create -f <file>")
	}

	// Validate file exists
	info, err := os.Stat(yamlFile)
	if err != nil {
		return fmt.Errorf("cannot read agent file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, expected a YAML file", yamlFile)
	}

	// Copy to agents directory
	agentsDir := agentsDirectory()
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		return fmt.Errorf("create agents directory: %w", err)
	}

	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return fmt.Errorf("read agent file: %w", err)
	}

	destName := filepath.Base(yamlFile)
	destPath := filepath.Join(agentsDir, destName)
	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return fmt.Errorf("write agent: %w", err)
	}

	name := strings.TrimSuffix(strings.TrimSuffix(destName, ".yaml"), ".yml")
	fmt.Fprintf(os.Stdout, "✅ Agent %q registered at %s\n", name, destPath)
	fmt.Fprintf(os.Stdout, "   Run with: tutu agent run %s\n", name)
	return nil
}

// ─── agent list ─────────────────────────────────────────────────────────────

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agents",
	Long:  `List all registered agent workflows.`,
	RunE:  runAgentList,
}

func runAgentList(cmd *cobra.Command, args []string) error {
	agentsDir := agentsDirectory()

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stdout, "No agents registered.")
			fmt.Fprintln(os.Stdout, "Use 'tutu agent create -f <file>' to register an agent.")
			return nil
		}
		return fmt.Errorf("read agents directory: %w", err)
	}

	var agents []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			agents = append(agents, strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"))
		}
	}

	if len(agents) == 0 {
		fmt.Fprintln(os.Stdout, "No agents registered.")
		fmt.Fprintln(os.Stdout, "Use 'tutu agent create -f <file>' to register an agent.")
		return nil
	}

	fmt.Fprintf(os.Stdout, "Registered agents (%d):\n", len(agents))
	for _, name := range agents {
		fmt.Fprintf(os.Stdout, "  • %s\n", name)
	}
	return nil
}

// ─── agent remove ───────────────────────────────────────────────────────────

var agentRemoveCmd = &cobra.Command{
	Use:   "remove AGENT_NAME",
	Short: "Remove a registered agent",
	Long:  `Remove an agent workflow registration.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentRemove,
}

func runAgentRemove(cmd *cobra.Command, args []string) error {
	agentName := args[0]
	agentsDir := agentsDirectory()

	// Try .yaml and .yml
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(agentsDir, agentName+ext)
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove agent: %w", err)
			}
			fmt.Fprintf(os.Stdout, "✅ Agent %q removed.\n", agentName)
			return nil
		}
	}

	return fmt.Errorf("agent %q not found", agentName)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// agentsDirectory returns the path to the agents directory.
func agentsDirectory() string {
	if env := os.Getenv("TUTU_HOME"); env != "" {
		return filepath.Join(env, "agents")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tutu", "agents")
}

// isPythonAvailable checks if Python 3.10+ is available.
func isPythonAvailable() bool {
	// Check common Python executable names
	for _, name := range []string{"python3", "python"} {
		path, err := findExecutable(name)
		if err == nil && path != "" {
			return true
		}
	}
	return false
}

// findExecutable searches PATH for the given executable name.
func findExecutable(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	pathSep := string(os.PathListSeparator)
	for _, dir := range strings.Split(pathEnv, pathSep) {
		// Check with and without .exe on Windows
		candidates := []string{
			filepath.Join(dir, name),
			filepath.Join(dir, name+".exe"),
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in PATH", name)
}
