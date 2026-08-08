package remediation

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"dzcli/internal/economy"

	"github.com/spf13/cobra"
)

var writeFileMutation = economy.WriteFileMutation

func NewGetEventsCommand(stdout io.Writer) *cobra.Command {
	var file string
	command := &cobra.Command{
		Use:   "events [name]",
		Short: "List db/events.xml event activity and positioning",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			entries, err := economy.ListEconomyEventsFile(file)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "NAME\tOCCURRENCE\tPOSITION\tACTIVE")
			matched := 0
			for _, entry := range entries {
				if filter != "" && entry.Name != filter {
					continue
				}
				matched++
				active := "<absent>"
				if entry.ActivePresent {
					active = strconv.Itoa(entry.Active)
				}
				fmt.Fprintf(writer, "%s\t%d\t%s\t%s\n", entry.Name, entry.Occurrence, entry.Position, active)
			}
			_ = writer.Flush()
			if matched == 0 {
				if filter != "" {
					return fmt.Errorf("event %q not found", filter)
				}
				return fmt.Errorf("no event resources found")
			}
			return nil
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "db/events.xml path")
	return command
}

func NewGetEventSpawnsCommand(stdout io.Writer) *cobra.Command {
	var file string
	command := &cobra.Command{
		Use:   "event-spawns [name]",
		Short: "List cfgeventspawns.xml event entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			entries, err := economy.ListEventSpawnsFile(file)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "NAME\tOCCURRENCE\tPOSITIONS\tZONES")
			matched := 0
			for _, entry := range entries {
				if filter != "" && entry.Name != filter {
					continue
				}
				matched++
				fmt.Fprintf(writer, "%s\t%d\t%s\t%s\n", entry.Name, entry.Occurrence, formatPositions(entry.Positions), formatZones(entry.Zones))
			}
			_ = writer.Flush()
			if matched == 0 {
				if filter != "" {
					return fmt.Errorf("event spawn %q not found", filter)
				}
				return fmt.Errorf("no event spawn resources found")
			}
			return nil
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "cfgeventspawns.xml path")
	return command
}

func NewCreateEventSpawnsCommand(stdout io.Writer) *cobra.Command {
	var file string
	var rawPositions []string
	var rawZone string
	var dryRun bool
	command := &cobra.Command{
		Use:   "event-spawns <name>",
		Short: "Create a cfgeventspawns.xml event entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			positions, err := parsePositions(rawPositions)
			if err != nil {
				return err
			}
			var zone *economy.EventSpawnZone
			if cmd.Flags().Changed("zone") {
				parsed, err := economy.ParseEventSpawnZone(rawZone)
				if err != nil {
					return err
				}
				zone = &parsed
			}
			mutation, err := economy.CreateEventSpawnFile(file, args[0], positions, zone)
			if err != nil {
				return err
			}
			return outputMutation(file, "event-spawns", mutation, dryRun, stdout)
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "cfgeventspawns.xml path")
	command.Flags().StringArrayVar(&rawPositions, "pos", nil, "position as x,z[,a[,y]]")
	command.Flags().StringVar(&rawZone, "zone", "", "zone as smin,smax,dmin,dmax,r")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "print modified XML without writing")
	return command
}

func NewUpdateEventSpawnsCommand(stdout io.Writer) *cobra.Command {
	var file string
	var occurrence int
	var rename string
	var rawSetPositions []string
	var rawAddPositions []string
	var removePositions []int
	var rawZone string
	var removeZone bool
	var dryRun bool
	command := &cobra.Command{
		Use:   "event-spawns <name>",
		Short: "Update one cfgeventspawns.xml event entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			setPositions, err := parsePositions(rawSetPositions)
			if err != nil {
				return err
			}
			addPositions, err := parsePositions(rawAddPositions)
			if err != nil {
				return err
			}
			options := economy.EventSpawnUpdateOptions{
				Name: args[0], Occurrence: occurrence, OccurrenceSet: cmd.Flags().Changed("occurrence"), Rename: rename,
				SetPositions: setPositions, SetPositionsSet: cmd.Flags().Changed("set-pos"), AddPositions: addPositions,
				RemovePositions: removePositions, RemoveZone: removeZone,
			}
			if cmd.Flags().Changed("set-zone") {
				zone, err := economy.ParseEventSpawnZone(rawZone)
				if err != nil {
					return err
				}
				options.SetZone = &zone
			}
			mutation, err := economy.UpdateEventSpawnFile(file, options)
			if err != nil {
				return err
			}
			return outputMutation(file, "event-spawns", mutation, dryRun, stdout)
		},
	}
	command.SetOut(stdout)
	flags := command.Flags()
	flags.StringVar(&file, "file", "", "cfgeventspawns.xml path")
	flags.IntVar(&occurrence, "occurrence", 0, "select a duplicate event occurrence")
	flags.StringVar(&rename, "rename", "", "rename the event entry")
	flags.StringArrayVar(&rawSetPositions, "set-pos", nil, "replace positions with x,z[,a[,y]]")
	flags.StringArrayVar(&rawAddPositions, "add-pos", nil, "add a position as x,z[,a[,y]]")
	flags.IntSliceVar(&removePositions, "remove-pos", nil, "remove a 1-based position occurrence")
	flags.StringVar(&rawZone, "set-zone", "", "replace the zone with smin,smax,dmin,dmax,r")
	flags.BoolVar(&removeZone, "remove-zone", false, "remove every zone")
	flags.BoolVar(&dryRun, "dry-run", false, "print modified XML without writing")
	return command
}

func NewDeleteEventSpawnsCommand(stdout io.Writer) *cobra.Command {
	var file string
	var occurrence int
	var dryRun bool
	command := &cobra.Command{
		Use:   "event-spawns <name>",
		Short: "Delete one cfgeventspawns.xml event entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			mutation, err := economy.DeleteEventSpawnFile(file, args[0], occurrence, cmd.Flags().Changed("occurrence"))
			if err != nil {
				return err
			}
			return outputMutation(file, "event-spawns", mutation, dryRun, stdout)
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "cfgeventspawns.xml path")
	command.Flags().IntVar(&occurrence, "occurrence", 0, "select a duplicate event occurrence")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "print modified XML without writing")
	return command
}

func NewGetEnvironmentCommand(stdout io.Writer) *cobra.Command {
	var file string
	command := &cobra.Command{
		Use:   "environment [territory-name]",
		Short: "List cfgenvironment.xml file references",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			refs, err := economy.ListEnvironmentReferencesFile(file)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "KIND\tVALUE\tTERRITORY\tOCCURRENCE\tTERRITORY-OCCURRENCE\tEXISTS")
			matched := 0
			for _, ref := range refs {
				if filter != "" && ref.Territory != filter {
					continue
				}
				matched++
				fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%d\t%t\n", ref.Kind, ref.Value, ref.Territory, ref.Occurrence, ref.TerritoryOccurrence, ref.Exists)
			}
			_ = writer.Flush()
			if matched == 0 {
				return fmt.Errorf("no environment references found")
			}
			return nil
		},
	}
	command.SetOut(stdout)
	command.Flags().StringVar(&file, "file", "", "cfgenvironment.xml path")
	return command
}

func NewCreateEnvironmentCommand(stdout io.Writer) *cobra.Command {
	return newEnvironmentCommand(economy.EnvironmentCreate, stdout)
}

func NewUpdateEnvironmentCommand(stdout io.Writer) *cobra.Command {
	return newEnvironmentCommand(economy.EnvironmentUpdate, stdout)
}

func NewDeleteEnvironmentCommand(stdout io.Writer) *cobra.Command {
	return newEnvironmentCommand(economy.EnvironmentDelete, stdout)
}

func newEnvironmentCommand(action economy.EnvironmentAction, stdout io.Writer) *cobra.Command {
	command := &cobra.Command{Use: "environment", Short: string(action) + " cfgenvironment.xml references", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	command.SetOut(stdout)
	command.AddCommand(newEnvironmentReferenceCommand(action, "path", stdout))
	command.AddCommand(newEnvironmentReferenceCommand(action, "usable", stdout))
	return command
}

func newEnvironmentReferenceCommand(action economy.EnvironmentAction, kind string, stdout io.Writer) *cobra.Command {
	var file string
	var occurrence int
	var territoryOccurrence int
	var replacementPath string
	var replacementUsable string
	var scaffold bool
	var dryRun bool
	use := kind + " <path>"
	if kind == "usable" {
		use = kind + " <territory> <usable>"
	}
	command := &cobra.Command{
		Use:   use,
		Short: string(action) + " an environment " + kind + " reference",
		Args: func(cmd *cobra.Command, args []string) error {
			if kind == "path" {
				return cobra.ExactArgs(1)(cmd, args)
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			options := economy.EnvironmentReferenceOptions{Kind: kind, Occurrence: occurrence, OccurrenceSet: cmd.Flags().Changed("occurrence"), TerritoryOccurrence: territoryOccurrence, TerritoryOccurrenceSet: cmd.Flags().Changed("territory-occurrence")}
			if kind == "path" {
				options.Value = args[0]
				options.Replacement = replacementPath
			} else {
				options.Territory = args[0]
				options.Value = args[1]
				options.Replacement = replacementUsable
			}
			missionRoot := filepath.Dir(filepath.Clean(file))
			pathToCheck := options.Value
			var scaffoldTarget string
			scaffoldCreated := false
			if action == economy.EnvironmentUpdate {
				pathToCheck = options.Replacement
			}
			if kind == "path" && action != economy.EnvironmentDelete {
				target, err := economy.ValidateEnvironmentRelativePath(missionRoot, pathToCheck)
				if err != nil {
					return err
				}
				if !scaffold {
					if info, err := os.Stat(target); err != nil {
						return fmt.Errorf("environment path %q does not exist; use --scaffold to create it: %w", pathToCheck, err)
					} else if !info.Mode().IsRegular() {
						return fmt.Errorf("environment path %q is not a regular file", pathToCheck)
					}
				}
			}
			mutation, err := economy.UpdateEnvironmentFile(file, action, options)
			if err != nil {
				return err
			}
			if dryRun {
				_, err = stdout.Write(mutation.Data)
				return err
			}
			if kind == "path" && action != economy.EnvironmentDelete && scaffold {
				var err error
				scaffoldTarget, scaffoldCreated, err = economy.ScaffoldTerritoryFileWithResult(missionRoot, pathToCheck)
				if err != nil {
					return err
				}
			}
			if err := writeFileMutation(file, mutation); err != nil {
				if scaffoldCreated {
					if rollbackErr := os.Remove(scaffoldTarget); rollbackErr != nil {
						return fmt.Errorf("write environment file: %w; rollback scaffold %s: %v", err, scaffoldTarget, rollbackErr)
					}
				}
				return err
			}
			fmt.Fprintf(stdout, "environment %s ok\n", file)
			return nil
		},
	}
	command.SetOut(stdout)
	flags := command.Flags()
	flags.StringVar(&file, "file", "", "cfgenvironment.xml path")
	flags.IntVar(&occurrence, "occurrence", 0, "select a duplicate reference occurrence")
	flags.IntVar(&territoryOccurrence, "territory-occurrence", 0, "select a duplicate territory occurrence")
	flags.BoolVar(&dryRun, "dry-run", false, "print modified XML without writing")
	if action == economy.EnvironmentUpdate && kind == "path" {
		flags.StringVar(&replacementPath, "set-path", "", "replace the registered path")
	}
	if action == economy.EnvironmentUpdate && kind == "usable" {
		flags.StringVar(&replacementUsable, "set-usable", "", "replace the usable file name")
	}
	if kind == "path" && action != economy.EnvironmentDelete {
		flags.BoolVar(&scaffold, "scaffold", false, "create a missing territory XML file")
	}
	return command
}

func parsePositions(values []string) ([]economy.EventSpawnPosition, error) {
	positions := make([]economy.EventSpawnPosition, 0, len(values))
	for _, value := range values {
		position, err := economy.ParseEventSpawnPosition(value)
		if err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return positions, nil
}

func formatPositions(values []economy.EventSpawnPosition) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		parts := []string{value.X, value.Z}
		if value.A != "" {
			parts = append(parts, value.A)
		}
		if value.Y != "" {
			parts = append(parts, value.Y)
		}
		result = append(result, strings.Join(parts, ","))
	}
	return strings.Join(result, ";")
}

func formatZones(values []economy.EventSpawnZone) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strings.Join([]string{value.SMin, value.SMax, value.DMin, value.DMax, value.R}, ","))
	}
	return strings.Join(result, ";")
}

func outputMutation(path string, kind string, mutation economy.FileMutation, dryRun bool, stdout io.Writer) error {
	if dryRun {
		_, err := stdout.Write(mutation.Data)
		return err
	}
	if err := writeFileMutation(path, mutation); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s %s ok\n", kind, path)
	return nil
}
