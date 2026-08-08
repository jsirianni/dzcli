package economy

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dzcli/internal/economyconfig"
)

// EventSpawnPosition is one explicit cfgeventspawns.xml event position.
type EventSpawnPosition struct {
	X string
	Z string
	A string
	Y string
}

// EventSpawnZone is the optional territory zone attached to an event spawn.
type EventSpawnZone struct {
	SMin string
	SMax string
	DMin string
	DMax string
	R    string
}

// EventSpawnEntry retains the information needed to inspect and target duplicate entries.
type EventSpawnEntry struct {
	Name       string
	Occurrence int
	Positions  []EventSpawnPosition
	Zones      []EventSpawnZone
}

type EconomyEventEntry struct {
	Name          string
	Occurrence    int
	Position      string
	Active        int
	ActivePresent bool
}

type EventSpawnUpdateOptions struct {
	Name            string
	Occurrence      int
	OccurrenceSet   bool
	Rename          string
	SetPositions    []EventSpawnPosition
	SetPositionsSet bool
	AddPositions    []EventSpawnPosition
	RemovePositions []int
	SetZone         *EventSpawnZone
	RemoveZone      bool
}

type EnvironmentReference struct {
	Kind                string
	Value               string
	Territory           string
	Occurrence          int
	TerritoryOccurrence int
	Exists              bool
}

type EnvironmentReferenceOptions struct {
	Kind                   string
	Value                  string
	Territory              string
	Replacement            string
	Occurrence             int
	OccurrenceSet          bool
	TerritoryOccurrence    int
	TerritoryOccurrenceSet bool
}

type EnvironmentAction string

const (
	EnvironmentCreate EnvironmentAction = "create"
	EnvironmentUpdate EnvironmentAction = "update"
	EnvironmentDelete EnvironmentAction = "delete"
)

func ParseEventSpawnPosition(value string) (EventSpawnPosition, error) {
	parts := splitCSV(value)
	if len(parts) < 2 || len(parts) > 4 {
		return EventSpawnPosition{}, fmt.Errorf("position expected x,z[,a[,y]], got %q", value)
	}
	for _, part := range parts {
		if !isFiniteNumber(part) {
			return EventSpawnPosition{}, fmt.Errorf("position value %q is not a number", part)
		}
	}
	position := EventSpawnPosition{X: parts[0], Z: parts[1]}
	if len(parts) > 2 {
		position.A = parts[2]
	}
	if len(parts) > 3 {
		position.Y = parts[3]
	}
	return position, nil
}

func ParseEventSpawnZone(value string) (EventSpawnZone, error) {
	parts := splitCSV(value)
	if len(parts) != 5 {
		return EventSpawnZone{}, fmt.Errorf("zone expected smin,smax,dmin,dmax,r, got %q", value)
	}
	for _, part := range parts {
		if !isFiniteNumber(part) {
			return EventSpawnZone{}, fmt.Errorf("zone value %q is not a number", part)
		}
	}
	return EventSpawnZone{SMin: parts[0], SMax: parts[1], DMin: parts[2], DMax: parts[3], R: parts[4]}, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func isFiniteNumber(value string) bool {
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func ListEventSpawnsFile(path string) ([]EventSpawnEntry, error) {
	data, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	return ParseEventSpawnEntriesData(data, path)
}

func ListEconomyEventsFile(path string) ([]EconomyEventEntry, error) {
	data, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	if _, err := economyconfig.ParseEventsData(data, path); err != nil {
		return nil, err
	}
	var document struct {
		Events []struct {
			Name     string `xml:"name,attr"`
			Position string `xml:"position"`
			Active   *int   `xml:"active"`
		} `xml:"event"`
	}
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	counts := map[string]int{}
	entries := make([]EconomyEventEntry, 0, len(document.Events))
	for _, event := range document.Events {
		counts[event.Name]++
		entry := EconomyEventEntry{Name: event.Name, Occurrence: counts[event.Name], Position: strings.TrimSpace(event.Position)}
		if event.Active != nil {
			entry.Active = *event.Active
			entry.ActivePresent = true
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func ParseEventSpawnEntriesData(data []byte, sourceName string) ([]EventSpawnEntry, error) {
	if _, err := economyconfig.ParseEventSpawnsData(data, sourceName); err != nil {
		return nil, err
	}
	ranges, err := findDirectNamedChildRanges(data, "event", "", "")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	entries := make([]EventSpawnEntry, 0, len(ranges))
	for _, target := range ranges {
		decoder := xml.NewDecoder(bytes.NewReader(data[target.Start:target.End]))
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "event" {
			return nil, fmt.Errorf("expected direct <event> child")
		}
		name := attributeValue(start, "name")
		counts[name]++
		entry := EventSpawnEntry{Name: name, Occurrence: counts[name]}
		if err := decodeEventSpawnEntry(decoder, start, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func decodeEventSpawnEntry(decoder *xml.Decoder, root xml.StartElement, entry *EventSpawnEntry) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "pos":
				entry.Positions = append(entry.Positions, EventSpawnPosition{X: attributeValue(typed, "x"), Z: attributeValue(typed, "z"), A: attributeValue(typed, "a"), Y: attributeValue(typed, "y")})
			case "zone":
				entry.Zones = append(entry.Zones, EventSpawnZone{SMin: attributeValue(typed, "smin"), SMax: attributeValue(typed, "smax"), DMin: attributeValue(typed, "dmin"), DMax: attributeValue(typed, "dmax"), R: attributeValue(typed, "r")})
			}
			if err := skipElement(decoder, typed.Name.Local); err != nil {
				return err
			}
		case xml.EndElement:
			if typed.Name.Local == root.Name.Local {
				return nil
			}
		}
	}
}

func CreateEventSpawnFile(path string, name string, positions []EventSpawnPosition, zone *EventSpawnZone) (FileMutation, error) {
	data, mode, err := readMutableFile(path)
	if err != nil {
		return FileMutation{}, err
	}
	updated, changed, err := CreateEventSpawnXML(data, name, positions, zone)
	return FileMutation{Data: updated, Mode: mode, Changed: changed}, err
}

func CreateEventSpawnXML(data []byte, name string, positions []EventSpawnPosition, zone *EventSpawnZone) ([]byte, bool, error) {
	if strings.TrimSpace(name) == "" {
		return nil, false, fmt.Errorf("event name is required")
	}
	if len(positions) == 0 && zone == nil {
		return nil, false, fmt.Errorf("event spawn requires at least one --pos or --zone")
	}
	entries, err := ParseEventSpawnEntriesData(data, "cfgeventspawns.xml")
	if err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return nil, false, fmt.Errorf("event spawn %q already exists", name)
		}
	}
	root, err := findElementRange(data, "eventposdef")
	if err != nil {
		return nil, false, err
	}
	rendered := renderEventSpawn(data, root, name, positions, zone)
	updated, err := insertBeforeElementClose(data, root, rendered)
	if err != nil {
		return nil, false, err
	}
	if _, err := ParseEventSpawnEntriesData(updated, "cfgeventspawns.xml"); err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func UpdateEventSpawnFile(path string, options EventSpawnUpdateOptions) (FileMutation, error) {
	data, mode, err := readMutableFile(path)
	if err != nil {
		return FileMutation{}, err
	}
	updated, changed, err := UpdateEventSpawnXML(data, options)
	return FileMutation{Data: updated, Mode: mode, Changed: changed}, err
}

func UpdateEventSpawnXML(data []byte, options EventSpawnUpdateOptions) ([]byte, bool, error) {
	target, err := selectDirectNamedRange(data, "event", "name", options.Name, options.Occurrence, options.OccurrenceSet)
	if err != nil {
		return nil, false, err
	}
	block := append([]byte(nil), data[target.Start:target.End]...)
	if options.Rename != "" {
		block, err = replaceAttribute(block, "name", options.Rename)
		if err != nil {
			return nil, false, err
		}
	}
	if options.SetPositionsSet {
		block, err = removeChildElements(block, "pos", nil)
		if err == nil {
			block, err = insertEventChildren(block, renderPositionLines(block, options.SetPositions))
		}
	}
	if err == nil && len(options.RemovePositions) > 0 {
		block, err = removeChildElements(block, "pos", options.RemovePositions)
	}
	if err == nil && len(options.AddPositions) > 0 {
		block, err = insertEventChildren(block, renderPositionLines(block, options.AddPositions))
	}
	if err == nil && options.RemoveZone {
		block, err = removeChildElements(block, "zone", nil)
	}
	if err == nil && options.SetZone != nil {
		block, err = removeChildElements(block, "zone", nil)
		if err == nil {
			block, err = insertEventChildren(block, renderZoneLines(block, *options.SetZone))
		}
	}
	if err != nil {
		return nil, false, err
	}
	parsed, err := ParseEventSpawnEntriesData(wrapEventSpawnBlock(block), "cfgeventspawns.xml")
	if err != nil {
		if strings.Contains(err.Error(), "requires at least one pos or zone") {
			return nil, false, fmt.Errorf("event spawn cannot be empty")
		}
		return nil, false, err
	}
	if len(parsed) != 1 || (len(parsed[0].Positions) == 0 && len(parsed[0].Zones) == 0) {
		return nil, false, fmt.Errorf("event spawn cannot be empty")
	}
	updated := replaceRange(data, target.Start, target.End, block)
	return updated, !bytes.Equal(data, updated), nil
}

func DeleteEventSpawnFile(path string, name string, occurrence int, occurrenceSet bool) (FileMutation, error) {
	data, mode, err := readMutableFile(path)
	if err != nil {
		return FileMutation{}, err
	}
	updated, changed, err := DeleteEventSpawnXML(data, name, occurrence, occurrenceSet)
	return FileMutation{Data: updated, Mode: mode, Changed: changed}, err
}

func DeleteEventSpawnXML(data []byte, name string, occurrence int, occurrenceSet bool) ([]byte, bool, error) {
	if _, err := ParseEventSpawnEntriesData(data, "cfgeventspawns.xml"); err != nil {
		return nil, false, err
	}
	target, err := selectDirectNamedRange(data, "event", "name", name, occurrence, occurrenceSet)
	if err != nil {
		return nil, false, err
	}
	start, end := expandWholeLine(data, target.Start, target.End)
	return replaceRange(data, start, end, nil), true, nil
}

func renderEventSpawn(document []byte, root elementRange, name string, positions []EventSpawnPosition, zone *EventSpawnZone) []byte {
	lineEnding := detectLineEnding(document)
	indent := detectElementIndent(document, root.Start) + "  "
	childIndent := indent + "  "
	var output strings.Builder
	output.WriteString(indent + `<event name="` + escapeAttribute(name) + `">` + lineEnding)
	for _, position := range positions {
		output.WriteString(childIndent + renderPosition(position) + lineEnding)
	}
	if zone != nil {
		output.WriteString(childIndent + renderZone(*zone) + lineEnding)
	}
	output.WriteString(indent + "</event>")
	return []byte(output.String())
}

func renderPosition(position EventSpawnPosition) string {
	value := `<pos x="` + escapeAttribute(position.X) + `" z="` + escapeAttribute(position.Z) + `"`
	if position.A != "" {
		value += ` a="` + escapeAttribute(position.A) + `"`
	}
	if position.Y != "" {
		value += ` y="` + escapeAttribute(position.Y) + `"`
	}
	return value + " />"
}

func renderZone(zone EventSpawnZone) string {
	return `<zone smin="` + escapeAttribute(zone.SMin) + `" smax="` + escapeAttribute(zone.SMax) + `" dmin="` + escapeAttribute(zone.DMin) + `" dmax="` + escapeAttribute(zone.DMax) + `" r="` + escapeAttribute(zone.R) + `" />`
}

func renderPositionLines(block []byte, positions []EventSpawnPosition) []byte {
	indent := detectChildIndent(block, "  ")
	lineEnding := detectLineEnding(block)
	var lines strings.Builder
	for _, position := range positions {
		lines.WriteString(indent + renderPosition(position) + lineEnding)
	}
	return []byte(lines.String())
}

func renderZoneLines(block []byte, zone EventSpawnZone) []byte {
	return []byte(detectChildIndent(block, "  ") + renderZone(zone) + detectLineEnding(block))
}

func insertEventChildren(block []byte, children []byte) ([]byte, error) {
	if len(children) == 0 {
		return block, nil
	}
	root, err := findElementRange(block, "event")
	if err != nil {
		return nil, err
	}
	return insertBeforeElementClose(block, root, bytes.TrimSuffix(children, []byte(detectLineEnding(block))))
}

func wrapEventSpawnBlock(block []byte) []byte {
	return append(append([]byte(`<eventposdef>`), block...), []byte(`</eventposdef>`)...)
}

func ListEnvironmentReferencesFile(path string) ([]EnvironmentReference, error) {
	data, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	refs, err := ParseEnvironmentReferencesData(data, path, filepath.Dir(filepath.Clean(path)))
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, ref := range refs {
		if ref.Kind == "path" && ref.Exists {
			existing[strings.TrimSuffix(filepath.Base(ref.Value), filepath.Ext(ref.Value))] = true
		}
	}
	for index := range refs {
		if refs[index].Kind == "usable" {
			refs[index].Exists = existing[refs[index].Value]
		}
	}
	return refs, nil
}

func ParseEnvironmentReferencesData(data []byte, sourceName string, missionRoot string) ([]EnvironmentReference, error) {
	if _, _, err := economyconfig.ParseEnvironmentData(data, sourceName); err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	type territoryState struct {
		Name       string
		Occurrence int
		Depth      int
	}
	var stack []territoryState
	territoryCounts := map[string]int{}
	refCounts := map[string]int{}
	var refs []EnvironmentReference
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return refs, nil
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
			if typed.Name.Local == "territory" {
				name := territoryName(typed)
				territoryCounts[name]++
				stack = append(stack, territoryState{Name: name, Occurrence: territoryCounts[name], Depth: depth})
			}
			if typed.Name.Local != "file" {
				continue
			}
			owner := territoryState{}
			if len(stack) > 0 {
				owner = stack[len(stack)-1]
			}
			if value := attributeValue(typed, "path"); value != "" {
				key := "path\x00" + value
				refCounts[key]++
				info, statErr := os.Stat(filepath.Join(missionRoot, filepath.FromSlash(value)))
				refs = append(refs, EnvironmentReference{Kind: "path", Value: value, Territory: owner.Name, Occurrence: refCounts[key], TerritoryOccurrence: owner.Occurrence, Exists: statErr == nil && info.Mode().IsRegular()})
			}
			if value := attributeValue(typed, "usable"); value != "" {
				key := "usable\x00" + owner.Name + "\x00" + value
				refCounts[key]++
				refs = append(refs, EnvironmentReference{Kind: "usable", Value: value, Territory: owner.Name, Occurrence: refCounts[key], TerritoryOccurrence: owner.Occurrence})
			}
		case xml.EndElement:
			if len(stack) > 0 && stack[len(stack)-1].Depth == depth && typed.Name.Local == "territory" {
				stack = stack[:len(stack)-1]
			}
			depth--
		}
	}
}

func UpdateEnvironmentFile(path string, action EnvironmentAction, options EnvironmentReferenceOptions) (FileMutation, error) {
	data, mode, err := readMutableFile(path)
	if err != nil {
		return FileMutation{}, err
	}
	updated, changed, err := UpdateEnvironmentXML(data, action, options)
	return FileMutation{Data: updated, Mode: mode, Changed: changed}, err
}

func UpdateEnvironmentXML(data []byte, action EnvironmentAction, options EnvironmentReferenceOptions) ([]byte, bool, error) {
	if options.Kind != "path" && options.Kind != "usable" {
		return nil, false, fmt.Errorf("environment reference kind must be path or usable")
	}
	if strings.TrimSpace(options.Value) == "" {
		return nil, false, fmt.Errorf("environment reference value is required")
	}
	if options.OccurrenceSet && options.Occurrence < 1 {
		return nil, false, fmt.Errorf("occurrence must be greater than 0")
	}
	if _, _, err := economyconfig.ParseEnvironmentData(data, "cfgenvironment.xml"); err != nil {
		return nil, false, err
	}
	var updated []byte
	var err error
	switch options.Kind {
	case "path":
		updated, err = mutateEnvironmentPath(data, action, options)
	case "usable":
		updated, err = mutateEnvironmentUsable(data, action, options)
	}
	if err != nil {
		return nil, false, err
	}
	if _, _, err := economyconfig.ParseEnvironmentData(updated, "cfgenvironment.xml"); err != nil {
		return nil, false, err
	}
	return updated, !bytes.Equal(data, updated), nil
}

func mutateEnvironmentPath(data []byte, action EnvironmentAction, options EnvironmentReferenceOptions) ([]byte, error) {
	if action == EnvironmentCreate {
		ranges, err := findNamedElementRanges(data, "file", "path", options.Value)
		if err != nil {
			return nil, err
		}
		if len(ranges) > 0 {
			return nil, fmt.Errorf("environment path %q already exists", options.Value)
		}
		parent, err := findElementRange(data, "territories")
		if err != nil {
			parent, err = findElementRange(data, "env")
		}
		if err != nil {
			return nil, err
		}
		indent := detectElementIndent(data, parent.Start) + "  "
		return insertBeforeElementClose(data, parent, []byte(indent+`<file path="`+escapeAttribute(options.Value)+`" />`))
	}
	target, err := selectNamedRange(data, "file", "path", options.Value, options.Occurrence, options.OccurrenceSet)
	if err != nil {
		return nil, err
	}
	if action == EnvironmentDelete {
		start, end := expandWholeLine(data, target.Start, target.End)
		return replaceRange(data, start, end, nil), nil
	}
	if action != EnvironmentUpdate || strings.TrimSpace(options.Replacement) == "" {
		return nil, fmt.Errorf("environment path update requires --set-path")
	}
	block, err := replaceAttribute(data[target.Start:target.End], "path", options.Replacement)
	if err != nil {
		return nil, err
	}
	return replaceRange(data, target.Start, target.End, block), nil
}

func mutateEnvironmentUsable(data []byte, action EnvironmentAction, options EnvironmentReferenceOptions) ([]byte, error) {
	territory, err := selectTerritoryRange(data, options.Territory, options.TerritoryOccurrence, options.TerritoryOccurrenceSet)
	if err != nil {
		return nil, err
	}
	block := append([]byte(nil), data[territory.Start:territory.End]...)
	if action == EnvironmentCreate {
		matches, err := findNamedElementRanges(block, "file", "usable", options.Value)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return nil, fmt.Errorf("usable file %q already exists in territory %q", options.Value, options.Territory)
		}
		root, _ := findElementRange(block, "territory")
		indent := detectElementIndent(block, root.Start) + "  "
		block, err = insertBeforeElementClose(block, root, []byte(indent+`<file usable="`+escapeAttribute(options.Value)+`" />`))
		if err != nil {
			return nil, err
		}
		return replaceRange(data, territory.Start, territory.End, block), nil
	}
	target, err := selectNamedRange(block, "file", "usable", options.Value, options.Occurrence, options.OccurrenceSet)
	if err != nil {
		return nil, err
	}
	if action == EnvironmentDelete {
		start, end := expandWholeLine(block, target.Start, target.End)
		block = replaceRange(block, start, end, nil)
	} else {
		if action != EnvironmentUpdate || strings.TrimSpace(options.Replacement) == "" {
			return nil, fmt.Errorf("environment usable update requires --set-usable")
		}
		replacement, replaceErr := replaceAttribute(block[target.Start:target.End], "usable", options.Replacement)
		if replaceErr != nil {
			return nil, replaceErr
		}
		block = replaceRange(block, target.Start, target.End, replacement)
	}
	return replaceRange(data, territory.Start, territory.End, block), nil
}

func ValidateEnvironmentRelativePath(missionRoot string, value string) (string, error) {
	for _, component := range strings.FieldsFunc(value, func(char rune) bool { return char == '/' || char == '\\' }) {
		if component == ".." {
			return "", fmt.Errorf("environment path must not contain a parent-directory component")
		}
	}
	if filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", fmt.Errorf("environment path must be relative to the mission root")
	}
	cleanSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleanSlash == "." || cleanSlash == ".." || strings.HasPrefix(cleanSlash, "../") {
		return "", fmt.Errorf("environment path must stay within the mission root")
	}
	rootAbs, err := filepath.Abs(missionRoot)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(cleanSlash)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("environment path must stay within the mission root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve mission root: %w", err)
	}
	ancestor := targetAbs
	for {
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("cannot resolve environment path parent")
		}
		ancestor = parent
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("resolve environment path: %w", err)
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolvedAncestor)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("environment path must stay within the mission root")
	}
	return targetAbs, nil
}

func ScaffoldTerritoryFile(missionRoot string, value string) error {
	_, _, err := ScaffoldTerritoryFileWithResult(missionRoot, value)
	return err
}

func ScaffoldTerritoryFileWithResult(missionRoot string, value string) (string, bool, error) {
	target, err := ValidateEnvironmentRelativePath(missionRoot, value)
	if err != nil {
		return "", false, err
	}
	if info, err := os.Stat(target); err == nil {
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("territory path %s is not a regular file", target)
		}
		return target, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", false, fmt.Errorf("create territory directory: %w", err)
	}
	data := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<territory-type />\n")
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- target is validated to remain within the mission root.
	if err != nil {
		if os.IsExist(err) {
			info, statErr := os.Stat(target)
			if statErr == nil && info.Mode().IsRegular() {
				return target, false, nil
			}
		}
		return "", false, fmt.Errorf("create territory file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return "", false, fmt.Errorf("write territory file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return "", false, fmt.Errorf("close territory file: %w", err)
	}
	return target, true, nil
}

func findNamedElementRanges(data []byte, elementName string, attributeName string, value string) ([]elementRange, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var ranges []elementRange
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return ranges, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != elementName {
			continue
		}
		startEnd := int(decoder.InputOffset())
		startOffset, err := findStartOffset(data, startEnd, elementName)
		if err != nil {
			return nil, err
		}
		endOffset, err := consumeElement(decoder, elementName)
		if err != nil {
			return nil, err
		}
		if attributeValue(start, attributeName) == value {
			ranges = append(ranges, elementRange{Start: startOffset, StartTagEnd: startEnd, End: endOffset})
		}
	}
}

func selectNamedRange(data []byte, elementName string, attributeName string, value string, occurrence int, occurrenceSet bool) (elementRange, error) {
	if strings.TrimSpace(value) == "" {
		return elementRange{}, fmt.Errorf("%s name is required", elementName)
	}
	if occurrenceSet && occurrence < 1 {
		return elementRange{}, fmt.Errorf("occurrence must be greater than 0")
	}
	ranges, err := findNamedElementRanges(data, elementName, attributeName, value)
	if err != nil {
		return elementRange{}, err
	}
	if len(ranges) == 0 {
		return elementRange{}, fmt.Errorf("%s %q not found", elementName, value)
	}
	if len(ranges) > 1 && !occurrenceSet {
		return elementRange{}, fmt.Errorf("%s %q appears %d times; use --occurrence to select one", elementName, value, len(ranges))
	}
	if occurrenceSet {
		if occurrence > len(ranges) {
			return elementRange{}, fmt.Errorf("%s %q occurrence %d not found", elementName, value, occurrence)
		}
		return ranges[occurrence-1], nil
	}
	return ranges[0], nil
}

func selectDirectNamedRange(data []byte, elementName string, attributeName string, value string, occurrence int, occurrenceSet bool) (elementRange, error) {
	if strings.TrimSpace(value) == "" {
		return elementRange{}, fmt.Errorf("%s name is required", elementName)
	}
	if occurrenceSet && occurrence < 1 {
		return elementRange{}, fmt.Errorf("occurrence must be greater than 0")
	}
	ranges, err := findDirectNamedChildRanges(data, elementName, attributeName, value)
	if err != nil {
		return elementRange{}, err
	}
	if len(ranges) == 0 {
		return elementRange{}, fmt.Errorf("%s %q not found", elementName, value)
	}
	if len(ranges) > 1 && !occurrenceSet {
		return elementRange{}, fmt.Errorf("%s %q appears %d times; use --occurrence to select one", elementName, value, len(ranges))
	}
	if occurrenceSet {
		if occurrence > len(ranges) {
			return elementRange{}, fmt.Errorf("%s %q occurrence %d not found", elementName, value, occurrence)
		}
		return ranges[occurrence-1], nil
	}
	return ranges[0], nil
}

func selectTerritoryRange(data []byte, name string, occurrence int, occurrenceSet bool) (elementRange, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var ranges []elementRange
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return elementRange{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "territory" {
			continue
		}
		startEnd := int(decoder.InputOffset())
		startOffset, err := findStartOffset(data, startEnd, "territory")
		if err != nil {
			return elementRange{}, err
		}
		end, err := consumeElement(decoder, "territory")
		if err != nil {
			return elementRange{}, err
		}
		if territoryName(start) == name {
			ranges = append(ranges, elementRange{Start: startOffset, StartTagEnd: startEnd, End: end})
		}
	}
	if len(ranges) == 0 {
		return elementRange{}, fmt.Errorf("territory %q not found", name)
	}
	if len(ranges) > 1 && !occurrenceSet {
		return elementRange{}, fmt.Errorf("territory %q appears %d times; use --territory-occurrence to select one", name, len(ranges))
	}
	if occurrenceSet {
		if occurrence < 1 || occurrence > len(ranges) {
			return elementRange{}, fmt.Errorf("territory %q occurrence %d not found", name, occurrence)
		}
		return ranges[occurrence-1], nil
	}
	return ranges[0], nil
}

func territoryName(start xml.StartElement) string {
	if name := attributeValue(start, "name"); name != "" {
		return name
	}
	return attributeValue(start, "type")
}

func insertBeforeElementClose(data []byte, target elementRange, content []byte) ([]byte, error) {
	closing := []byte("</")
	relative := bytes.LastIndex(data[target.Start:target.End], closing)
	if relative < 0 {
		block := data[target.Start:target.End]
		decoder := xml.NewDecoder(bytes.NewReader(block))
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			return nil, fmt.Errorf("cannot identify self-closing element")
		}
		trimmed := strings.TrimRight(string(block), " \t\r\n")
		opening := strings.TrimRight(strings.TrimSuffix(trimmed, "/>"), " \t\r\n") + ">"
		lineEnding := detectLineEnding(data)
		indent := detectElementIndent(data, target.Start)
		replacement := []byte(opening + lineEnding + string(content) + lineEnding + indent + "</" + start.Name.Local + ">")
		return replaceRange(data, target.Start, target.End, replacement), nil
	}
	position := target.Start + relative
	lineEnding := detectLineEnding(data)
	indent := detectElementIndent(data, target.Start)
	lineStart := bytes.LastIndexByte(data[:position], '\n') + 1
	if strings.TrimSpace(string(data[lineStart:position])) == "" {
		return replaceRange(data, lineStart, lineStart, append(content, []byte(lineEnding)...)), nil
	}
	replacement := []byte(lineEnding + string(content) + lineEnding + indent)
	return replaceRange(data, position, position, replacement), nil
}

func replaceAttribute(block []byte, name string, value string) ([]byte, error) {
	pattern := regexp.MustCompile(`(?s)(\s` + regexp.QuoteMeta(name) + `\s*=\s*)(["'])(.*?)(["'])`)
	location := pattern.FindSubmatchIndex(block)
	if location == nil || !bytes.Equal(block[location[4]:location[5]], block[location[8]:location[9]]) {
		return nil, fmt.Errorf("attribute %q not found", name)
	}
	replacement := append([]byte(nil), block[:location[6]]...)
	replacement = append(replacement, []byte(escapeAttribute(value))...)
	replacement = append(replacement, block[location[7]:]...)
	return replacement, nil
}

func removeChildElements(block []byte, name string, occurrences []int) ([]byte, error) {
	ranges, err := findDirectChildRanges(block, name)
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return block, nil
	}
	removeAll := occurrences == nil
	wanted := map[int]bool{}
	for _, occurrence := range occurrences {
		if occurrence < 1 || occurrence > len(ranges) {
			return nil, fmt.Errorf("%s occurrence %d not found", name, occurrence)
		}
		wanted[occurrence] = true
	}
	sort.Slice(ranges, func(left, right int) bool { return ranges[left].Start > ranges[right].Start })
	for reverseIndex, target := range ranges {
		originalIndex := len(ranges) - reverseIndex
		if !removeAll && !wanted[originalIndex] {
			continue
		}
		start, end := expandWholeLine(block, target.Start, target.End)
		block = replaceRange(block, start, end, nil)
	}
	return block, nil
}

func findDirectChildRanges(block []byte, childName string) ([]elementRange, error) {
	decoder := xml.NewDecoder(bytes.NewReader(block))
	depth := 0
	var ranges []elementRange
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return ranges, nil
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if depth == 1 && typed.Name.Local == childName {
				startEnd := int(decoder.InputOffset())
				startOffset, err := findStartOffset(block, startEnd, childName)
				if err != nil {
					return nil, err
				}
				end, err := consumeElement(decoder, childName)
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, elementRange{Start: startOffset, StartTagEnd: startEnd, End: end})
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

func findDirectNamedChildRanges(block []byte, childName string, attributeName string, value string) ([]elementRange, error) {
	ranges, err := findDirectChildRanges(block, childName)
	if err != nil {
		return nil, err
	}
	if attributeName == "" {
		return ranges, nil
	}
	filtered := make([]elementRange, 0, len(ranges))
	for _, target := range ranges {
		decoder := xml.NewDecoder(bytes.NewReader(block[target.Start:target.StartTagEnd]))
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if ok && attributeValue(start, attributeName) == value {
			filtered = append(filtered, target)
		}
	}
	return filtered, nil
}

func readConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- command intentionally reads a user-selected config file.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}
