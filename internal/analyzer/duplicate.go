package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"os"
	"sort"
	"strings"
)

const (
	defaultDuplicateWindowLines   = 12
	defaultDuplicateWindowTokens  = 100
	defaultDuplicateTokenMinLines = 12
	defaultDuplicateMinStatements = 3

	minimumDuplicateWindowLines   = 6
	minimumDuplicateWindowTokens  = 50
	minimumDuplicateTokenMinLines = 8
	minimumDuplicateMinStatements = 1
	duplicateHashLength           = 12
)

type DuplicateOptions struct {
	MinLines      int
	MinTokens     int
	MinTokenLines int
	MinStatements int
}

func (o DuplicateOptions) Validate() error {
	for _, threshold := range []struct {
		name, minimum string
		value         int
		minimumValue  int
	}{
		{name: "duplicate minimum lines", minimum: "6", value: o.MinLines, minimumValue: minimumDuplicateWindowLines},
		{name: "duplicate minimum tokens", minimum: "50", value: o.MinTokens, minimumValue: minimumDuplicateWindowTokens},
		{name: "duplicate minimum token lines", minimum: "8", value: o.MinTokenLines, minimumValue: minimumDuplicateTokenMinLines},
		{name: "duplicate minimum statements", minimum: "1", value: o.MinStatements, minimumValue: minimumDuplicateMinStatements},
	} {
		if threshold.value != 0 && threshold.value < threshold.minimumValue {
			return fmt.Errorf("%s must be zero or at least %s", threshold.name, threshold.minimum)
		}
	}
	return nil
}

func (o DuplicateOptions) normalized() DuplicateOptions {
	if o.MinLines == 0 {
		o.MinLines = defaultDuplicateWindowLines
	}
	if o.MinTokens == 0 {
		o.MinTokens = defaultDuplicateWindowTokens
	}
	if o.MinTokenLines == 0 {
		o.MinTokenLines = defaultDuplicateTokenMinLines
	}
	if o.MinStatements == 0 {
		o.MinStatements = defaultDuplicateMinStatements
	}
	return o
}

const (
	duplicateLineKeyPrefix  = "line:"
	duplicateTokenKeyPrefix = "token:"
)

type duplicateWindow struct {
	file      string
	startLine int
	endLine   int
}

type duplicateGroup struct {
	key       string
	locations []duplicateWindow
}

func duplicateFindings(module moduleState, options DuplicateOptions) []Finding {
	options = options.normalized()
	windows := make(map[string][]duplicateWindow)
	for _, pkg := range module.packages {
		for _, source := range pkg.files {
			data, err := os.ReadFile(source.absPath)
			if err != nil {
				continue
			}
			statements := meaningfulStatementSpans(source.file, source.fset)
			collectDuplicateWindows(windows, source.relPath, string(data), statements, options)
			collectTokenDuplicateWindows(windows, source.relPath, data, source.file, source.fset, statements, options)
		}
	}

	keys := make([]string, 0, len(windows))
	for key, locations := range windows {
		if len(locations) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	groups := make([]duplicateGroup, 0, len(keys))
	for _, key := range keys {
		locations := dedupeDuplicateWindows(windows[key])
		if len(locations) < 2 {
			continue
		}
		groups = append(groups, duplicateGroup{key: key, locations: locations})
	}

	groups = coalesceDuplicateGroups(groups)
	groups = removeOverlappingDuplicateGroups(groups)
	findings := make([]Finding, 0, len(groups))
	for _, group := range groups {
		locations := group.locations
		first := locations[0]
		findingLocations := make([]Location, 0, len(locations))
		for _, loc := range locations {
			findingLocations = append(findingLocations, Location{
				File:      loc.file,
				StartLine: loc.startLine,
				EndLine:   loc.endLine,
			})
		}

		findings = append(findings, Finding{
			Type:        FindingDuplicateCode,
			Symbol:      duplicateSymbol(group.key),
			File:        first.file,
			Line:        first.startLine,
			Lines:       duplicatedLineCount(locations),
			Locations:   findingLocations,
			Fingerprint: "",
		})
	}

	return findings
}

func removeOverlappingDuplicateGroups(groups []duplicateGroup) []duplicateGroup {
	filtered := make([]duplicateGroup, 0, len(groups))
	for _, group := range groups {
		if overlapsAnyDuplicateGroup(group, filtered) {
			continue
		}
		filtered = append(filtered, group)
	}

	return filtered
}

func overlapsAnyDuplicateGroup(group duplicateGroup, groups []duplicateGroup) bool {
	for _, existing := range groups {
		if duplicateGroupsOverlap(existing, group) {
			return true
		}
	}

	return false
}

func duplicateGroupsOverlap(left duplicateGroup, right duplicateGroup) bool {
	for _, leftLocation := range left.locations {
		for _, rightLocation := range right.locations {
			if duplicateWindowsOverlap(leftLocation, rightLocation) {
				return true
			}
		}
	}

	return false
}

func duplicateWindowsOverlap(left duplicateWindow, right duplicateWindow) bool {
	if left.file != right.file {
		return false
	}
	return left.startLine <= right.endLine && right.startLine <= left.endLine
}

func coalesceDuplicateGroups(groups []duplicateGroup) []duplicateGroup {
	sort.Slice(groups, func(i, j int) bool {
		left := groups[i].locations[0]
		right := groups[j].locations[0]
		if left.file == right.file {
			return left.startLine < right.startLine
		}
		return left.file < right.file
	})

	coalesced := make([]duplicateGroup, 0, len(groups))
	for _, group := range groups {
		lastIndex := len(coalesced) - 1
		if lastIndex >= 0 && canMergeDuplicateGroups(coalesced[lastIndex], group) {
			mergeDuplicateGroup(&coalesced[lastIndex], group)
			continue
		}
		coalesced = append(coalesced, group)
	}

	return coalesced
}

func canMergeDuplicateGroups(left duplicateGroup, right duplicateGroup) bool {
	if len(left.locations) != len(right.locations) {
		return false
	}
	for i := range left.locations {
		leftLocation := left.locations[i]
		rightLocation := right.locations[i]
		if leftLocation.file != rightLocation.file {
			return false
		}
		if rightLocation.startLine < leftLocation.startLine || rightLocation.startLine > leftLocation.endLine+1 {
			return false
		}
	}

	return true
}

func mergeDuplicateGroup(left *duplicateGroup, right duplicateGroup) {
	for i := range left.locations {
		if right.locations[i].endLine > left.locations[i].endLine {
			left.locations[i].endLine = right.locations[i].endLine
		}
	}
}

func duplicatedLineCount(locations []duplicateWindow) int {
	if len(locations) < 2 {
		return 0
	}
	return (locations[0].endLine - locations[0].startLine + 1) * (len(locations) - 1)
}

func collectDuplicateWindows(windows map[string][]duplicateWindow, file string, content string, statements []meaningfulStatement, options DuplicateOptions) {
	lines := strings.Split(content, "\n")
	normalized := make([]string, 0, len(lines))
	lineNumbers := make([]int, 0, len(lines))
	for i, line := range lines {
		line = normalizeDuplicateLine(line)
		if line == "" {
			continue
		}
		normalized = append(normalized, line)
		lineNumbers = append(lineNumbers, i+1)
	}
	if len(normalized) < options.MinLines {
		return
	}

	for i := 0; i <= len(normalized)-options.MinLines; i++ {
		startLine := lineNumbers[i]
		endLine := lineNumbers[i+options.MinLines-1]
		if !duplicateWindowHasMeaningfulStatements(statements, startLine, endLine, options.MinStatements) {
			continue
		}
		chunk := strings.Join(normalized[i:i+options.MinLines], "\n")
		key := duplicateKey(duplicateLineKeyPrefix + chunk)
		windows[key] = append(windows[key], duplicateWindow{
			file:      file,
			startLine: startLine,
			endLine:   endLine,
		})
	}
}

type duplicateToken struct {
	value string
	line  int
}

func collectTokenDuplicateWindows(windows map[string][]duplicateWindow, file string, content []byte, syntax *ast.File, fset *token.FileSet, statements []meaningfulStatement, options DuplicateOptions) {
	tokens := duplicateTokens(file, content)
	for _, body := range functionBodyLineRanges(syntax, fset) {
		collectTokenSequenceDuplicateWindows(windows, file, tokensInLineRange(tokens, body), statements, options)
	}
}

func duplicateTokens(file string, content []byte) []duplicateToken {
	fset := token.NewFileSet()
	tokenFile := fset.AddFile(file, fset.Base(), len(content))
	var scan scanner.Scanner
	scan.Init(tokenFile, content, nil, 0)

	tokens := make([]duplicateToken, 0)
	previous := token.ILLEGAL
	for {
		pos, tok, _ := scan.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON {
			continue
		}
		value, ok := normalizeDuplicateToken(tok, previous)
		if !ok {
			continue
		}
		tokens = append(tokens, duplicateToken{
			value: value,
			line:  fset.Position(pos).Line,
		})
		previous = tok
	}

	return tokens
}

func functionBodyLineRanges(syntax *ast.File, fset *token.FileSet) []duplicateWindow {
	if syntax == nil || fset == nil {
		return nil
	}

	ranges := make([]duplicateWindow, 0)
	for _, decl := range syntax.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ranges = append(ranges, duplicateWindow{
			startLine: fset.Position(fn.Body.Lbrace).Line,
			endLine:   fset.Position(fn.Body.Rbrace).Line,
		})
	}

	return ranges
}

func tokensInLineRange(tokens []duplicateToken, lines duplicateWindow) []duplicateToken {
	filtered := make([]duplicateToken, 0)
	for _, tok := range tokens {
		if tok.line < lines.startLine || tok.line > lines.endLine {
			continue
		}
		filtered = append(filtered, tok)
	}

	return filtered
}

func collectTokenSequenceDuplicateWindows(windows map[string][]duplicateWindow, file string, tokens []duplicateToken, statements []meaningfulStatement, options DuplicateOptions) {
	if len(tokens) < options.MinTokens {
		return
	}

	for i := 0; i <= len(tokens)-options.MinTokens; i++ {
		end := i + options.MinTokens - 1
		startLine := tokens[i].line
		endLine := tokens[end].line
		if duplicateTokenLineCount(tokens[i:i+options.MinTokens]) < options.MinTokenLines || !duplicateWindowHasMeaningfulStatements(statements, startLine, endLine, options.MinStatements) {
			continue
		}

		key := duplicateKey(duplicateTokenKeyPrefix + duplicateTokenChunk(tokens[i:i+options.MinTokens]))
		windows[key] = append(windows[key], duplicateWindow{
			file:      file,
			startLine: startLine,
			endLine:   endLine,
		})
	}
}

func duplicateTokenLineCount(tokens []duplicateToken) int {
	previousLine := -1
	count := 0
	for _, token := range tokens {
		if token.line == previousLine {
			continue
		}
		previousLine = token.line
		count++
	}
	return count
}

type meaningfulStatement struct {
	startLine int
	endLine   int
}

func meaningfulStatementSpans(syntax *ast.File, fset *token.FileSet) []meaningfulStatement {
	if syntax == nil || fset == nil {
		return nil
	}

	spans := make([]meaningfulStatement, 0)
	ast.Inspect(syntax, func(node ast.Node) bool {
		statement, ok := node.(ast.Stmt)
		if !ok {
			return true
		}
		if guard, ok := statement.(*ast.IfStmt); ok && isSingleReturnGuard(guard) {
			return false
		}
		if !isMeaningfulStatement(statement) {
			return true
		}
		spans = append(spans, meaningfulStatement{
			startLine: fset.Position(statement.Pos()).Line,
			endLine:   fset.Position(statement.End()).Line,
		})
		return true
	})

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].startLine == spans[j].startLine {
			return spans[i].endLine < spans[j].endLine
		}
		return spans[i].startLine < spans[j].startLine
	})
	return spans
}

func isMeaningfulStatement(statement ast.Stmt) bool {
	if guard, ok := statement.(*ast.IfStmt); ok && isSingleReturnGuard(guard) {
		return false
	}

	switch statement := statement.(type) {
	case *ast.AssignStmt, *ast.IncDecStmt, *ast.SendStmt, *ast.GoStmt, *ast.RangeStmt, *ast.ForStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.IfStmt:
		return true
	case *ast.ExprStmt:
		_, ok := statement.X.(*ast.CallExpr)
		return ok
	default:
		return false
	}
}

func duplicateWindowHasMeaningfulStatements(statements []meaningfulStatement, startLine, endLine, minimum int) bool {
	if minimum < 1 {
		return false
	}

	first := sort.Search(len(statements), func(index int) bool {
		return statements[index].startLine >= startLine
	})
	count := 0
	for _, statement := range statements[first:] {
		if statement.startLine > endLine {
			break
		}
		if statement.endLine > endLine {
			continue
		}
		count++
		if count >= minimum {
			return true
		}
	}
	return false
}

func isSingleReturnGuard(statement *ast.IfStmt) bool {
	if statement.Else != nil || statement.Body == nil || len(statement.Body.List) != 1 {
		return false
	}
	_, ok := statement.Body.List[0].(*ast.ReturnStmt)
	return ok
}

func normalizeDuplicateToken(tok token.Token, previous token.Token) (string, bool) {
	switch {
	case tok == token.IDENT:
		if previous == token.PERIOD {
			return "FIELD", true
		}
		return tok.String(), true
	case tok.IsLiteral():
		return tok.String(), true
	case tok == token.ILLEGAL || tok == token.COMMENT:
		return "", false
	default:
		return tok.String(), true
	}
}

func duplicateTokenChunk(tokens []duplicateToken) string {
	values := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		values = append(values, tok.value)
	}

	return strings.Join(values, " ")
}

func normalizeDuplicateLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "//") {
		return ""
	}
	return strings.Join(strings.Fields(line), " ")
}

func duplicateKey(chunk string) string {
	sum := sha256.Sum256([]byte(chunk))
	return hex.EncodeToString(sum[:])
}

func duplicateSymbol(key string) string {
	return fmt.Sprintf("clone-%s", key[:duplicateHashLength])
}

func dedupeDuplicateWindows(locations []duplicateWindow) []duplicateWindow {
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].file == locations[j].file {
			return locations[i].startLine < locations[j].startLine
		}
		return locations[i].file < locations[j].file
	})

	seenFiles := make(map[string]struct{}, len(locations))
	deduped := make([]duplicateWindow, 0, len(locations))
	for _, location := range locations {
		key := fmt.Sprintf("%s:%d", location.file, location.startLine)
		if _, ok := seenFiles[key]; ok {
			continue
		}
		seenFiles[key] = struct{}{}
		deduped = append(deduped, location)
	}

	return mergeOverlappingDuplicateWindows(deduped)
}

func mergeOverlappingDuplicateWindows(locations []duplicateWindow) []duplicateWindow {
	merged := make([]duplicateWindow, 0, len(locations))
	for _, location := range locations {
		lastIndex := len(merged) - 1
		if lastIndex < 0 || !duplicateWindowsCanMerge(merged[lastIndex], location) {
			merged = append(merged, location)
			continue
		}
		if location.endLine > merged[lastIndex].endLine {
			merged[lastIndex].endLine = location.endLine
		}
	}

	return merged
}

func duplicateWindowsCanMerge(left duplicateWindow, right duplicateWindow) bool {
	return left.file == right.file && right.startLine <= left.endLine+1
}
