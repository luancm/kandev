package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// promptTaskCallers pins every function that calls promptTask. Every parent
// prompt reaches promptTask, which composes the errand status block. A new
// caller must be added here on purpose, after confirming it does not bypass
// composition.
var promptTaskCallers = map[string]struct{}{
	"PromptTask":                              {},
	"PromptTaskWithDeliverySubmissionID":      {},
	"PromptUnattendedWake":                    {},
	"RecoverSessionWithOptions":               {},
	"StartSessionForWorkflowStep":             {},
	"dispatchCIAutomationPromptToIdleSession": {},
	"executeQueuedMessageWithReservation":     {},
	"launchPreparedDynamicRelaunch":           {},
	"promptAutomationContinuation":            {},
	"promptSendNowClaim":                      {},
	"promptTaskWithPromptContext":             {},
	"replayCeilingLaunchPromptEnsure":         {},
	"replayInitialPromptSubmission":           {},
	"resumeClarificationViaFallback":          {},
	"resumeDetachedClarificationWithPrompt":   {},
	"resumeTaskSessionAndPrompt":              {},
	"retryAutoStartStepPrompt":                {},
	"retryInterruptedContinuation":            {},
	"retryRetainedRuntimeContinuation":        {},
	"retryRetainedRuntimePrompt":              {},
	"retryTransientPrompt":                    {},
}

// TestPromptTaskCallersAreComposed fails when a non-test file calls promptTask
// from a function that is not in promptTaskCallers.
func TestPromptTaskCallersAreComposed(t *testing.T) {
	found := promptTaskCallerNames(t)
	var unpinned []string
	for name := range found {
		if _, ok := promptTaskCallers[name]; !ok {
			unpinned = append(unpinned, name)
		}
	}
	sort.Strings(unpinned)
	require.Empty(t, unpinned, "new promptTask callers must be pinned in promptTaskCallers")
}

func promptTaskCallerNames(t *testing.T) map[string]struct{} {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	found := map[string]struct{}{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		collectPromptTaskCallers(parsed, found)
	}
	return found
}

func collectPromptTaskCallers(file *ast.File, found map[string]struct{}) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "promptTask" {
				found[fn.Name.Name] = struct{}{}
			}
			return true
		})
	}
}
