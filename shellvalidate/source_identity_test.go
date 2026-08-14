package shellvalidate

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestFilenameIdentityRemainsOpaque(t *testing.T) {
	identities := []string{
		`dir\script.sh`,
		"dir/script.sh",
		"dir/./script.sh",
		"dir/child/../script.sh",
		`dir\child\..\script.sh`,
	}
	for _, identity := range identities {
		identity := identity
		t.Run(identity, func(t *testing.T) {
			file, diagnostics, err := Parse(identity, []byte(":\n"), DialectPOSIX)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("Parse() = %v, %#v", err, diagnostics)
			}
			if got := file.Filename(); got != identity {
				t.Fatalf("Filename() = %q, want opaque identity %q", got, identity)
			}
		})
	}
}

type resolverCall struct {
	from      string
	requested string
}

type opaqueIdentityResolver struct {
	calls      []resolverCall
	identities [2]string
}

func (resolver *opaqueIdentityResolver) Resolve(_ context.Context, fromFilename, requestedPath string) (string, []byte, error) {
	resolver.calls = append(resolver.calls, resolverCall{from: fromFilename, requested: requestedPath})
	switch len(resolver.calls) {
	case 1:
		return resolver.identities[0], []byte(". two\n"), nil
	case 2:
		return resolver.identities[1], []byte("break\n"), nil
	default:
		return "", nil, fmt.Errorf("unexpected resolver call %d", len(resolver.calls))
	}
}

func TestResolverFilenamesAreOpaqueCanonicalIdentities(t *testing.T) {
	cases := map[string][2]string{
		"forward-slash": {"tree/a/../child", "tree/child"},
		"backslash":     {`tree\a\..\child`, `tree\child`},
		"explicit-dot":  {"tree/./child", "tree/child"},
		"backslash-dot": {`tree\.\child`, `tree\child`},
	}
	for name, identities := range cases {
		t.Run(name, func(t *testing.T) {
			resolver := &opaqueIdentityResolver{identities: identities}
			result, err := Check(t.Context(), `root\.\entry.sh`, []byte(". one\n"), Options{
				Dialect: DialectPOSIX, AnalyzeSourced: true, Resolver: resolver,
			})
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := []resolverCall{
				{from: `root\.\entry.sh`, requested: "one"},
				{from: identities[0], requested: "two"},
			}
			if !reflect.DeepEqual(resolver.calls, wantCalls) {
				t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, wantCalls)
			}
			if !result.AnalysisExact || hasCode(result.Diagnostics, "SHI1001") {
				t.Fatalf("path-like opaque identities were treated as a cycle: %#v", result)
			}
			if !hasCode(result.Diagnostics, "SHC1001") {
				t.Fatalf("distinct second source was not analyzed: %#v", result.Diagnostics)
			}
		})
	}
}
