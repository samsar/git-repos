package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samsar/git-repos/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage git-repos configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.Path()
		fmt.Printf("Config file: %s\n\n", path)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("(no config file yet — run git-repos to create one)")
				return nil
			}
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

var configAddCmd = &cobra.Command{
	Use:   "add <dir>",
	Short: "Add a directory to scan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		abs, err := filepath.Abs(config.ExpandHome(args[0]))
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("directory not found: %s", abs)
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		for _, d := range cfg.Dirs {
			if d == abs {
				fmt.Printf("%s is already in the config\n", abs)
				return nil
			}
		}
		cfg.Dirs = append(cfg.Dirs, abs)
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Added %s\n", abs)
		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the config file (will prompt on next run)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.Path()
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No config file to remove")
				return nil
			}
			return err
		}
		fmt.Printf("Removed %s\n", path)
		return nil
	},
}

var configHideCmd = &cobra.Command{
	Use:   "hide <name>",
	Short: "Hide a repo from the output by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		for _, h := range cfg.Hidden {
			if h == name {
				fmt.Printf("%s is already hidden\n", name)
				return nil
			}
		}
		cfg.Hidden = append(cfg.Hidden, name)
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Hidden: %s\n", name)
		return nil
	},
}

var configUnhideCmd = &cobra.Command{
	Use:   "unhide <name>",
	Short: "Stop hiding a repo by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg == nil {
			fmt.Printf("%s is not hidden\n", name)
			return nil
		}
		next := cfg.Hidden[:0]
		found := false
		for _, h := range cfg.Hidden {
			if h == name {
				found = true
			} else {
				next = append(next, h)
			}
		}
		if !found {
			fmt.Printf("%s is not hidden\n", name)
			return nil
		}
		cfg.Hidden = next
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Unhidden: %s\n", name)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configAddCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configHideCmd)
	configCmd.AddCommand(configUnhideCmd)
}
