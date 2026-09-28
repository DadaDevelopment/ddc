package agentrun

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/dada-tuda/ddc/internal/agentspec"
)

// headerEnvRef is a ${VAR} reference inside a header value. The platform
// resolves the same syntax from the deployed agent's env, so one manifest
// header ("Bearer ${TOOLS_TOKEN}") works locally and in production while the
// token stays out of the repo.
var headerEnvRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// localAgentPy serves one agent from the mounted config dir inside the prod
// image. It is embedded so `ddc agent up` needs nothing but the binary.
//
//go:embed local_agent.py
var localAgentPy []byte

// ToolResolver asks the console which tools an agent runs with. cliapp supplies
// the real one; keeping it an interface here means this package never reaches
// for a session or an HTTP client of its own.
type ToolResolver interface {
	AgentTools(agent string) ([]agentspec.Tool, error)
	AppURL(app string) (string, error)
}

// resolveTools decides which MCP servers the agent may call, in priority order:
// the --mcp flag, then the tools declared in the manifest, then the console.
func resolveTools(spec *agentspec.Spec, opts Options) ([]agentspec.Tool, error) {
	if len(opts.MCP) > 0 {
		tools := make([]agentspec.Tool, 0, len(opts.MCP))
		for _, u := range opts.MCP {
			tools = append(tools, agentspec.Tool{URL: u})
		}
		return tools, nil
	}
	if len(spec.Runtime.Tools) > 0 {
		tools := make([]agentspec.Tool, 0, len(spec.Runtime.Tools))
		for _, t := range spec.Runtime.Tools {
			headers, err := toolHeaders(t)
			if err != nil {
				return nil, err
			}
			tools = append(tools, agentspec.Tool{URL: t.URL, Headers: headers, Timeout: t.Timeout})
		}
		return tools, nil
	}
	if opts.Resolver == nil {
		return nil, fmt.Errorf("no tools: declare them in %s or pass --mcp URL", agentspec.OverridePath)
	}
	return consoleTools(spec, opts.Resolver)
}

// toolHeaders merges literal headers with any carried in an environment
// variable, so a private MCP server's credentials stay out of the repo. A
// ${VAR} inside a literal header is taken from the local environment (or
// .ddc/.env); an unset one fails loudly, because a local agent without its
// tool credentials runs toolless and every eval against it is meaningless.
func toolHeaders(t agentspec.Tool) (map[string]string, error) {
	headers := map[string]string{}
	for k, v := range t.Headers {
		var missing []string
		headers[k] = headerEnvRef.ReplaceAllStringFunc(v, func(ref string) string {
			name := headerEnvRef.FindStringSubmatch(ref)[1]
			value, ok := os.LookupEnv(name)
			if !ok || value == "" {
				missing = append(missing, name)
			}
			return value
		})
		if len(missing) > 0 {
			return nil, fmt.Errorf("tool %s header %s needs %s in the environment or .ddc/.env", t.URL, k, strings.Join(missing, ", "))
		}
	}
	if t.HeadersEnv == "" {
		return headers, nil
	}
	raw := os.Getenv(t.HeadersEnv)
	if raw == "" {
		return nil, fmt.Errorf("tool %s declares headers_env %s but it is not set", t.URL, t.HeadersEnv)
	}
	for _, line := range strings.Split(raw, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return headers, nil
}

// consoleTools asks the console which tools this agent runs with in its
// environment, rewriting in-cluster URLs to the app's public URL.
func consoleTools(spec *agentspec.Spec, resolver ToolResolver) ([]agentspec.Tool, error) {
	tools, err := resolver.AgentTools(spec.Name)
	if err != nil {
		return nil, err
	}
	out := make([]agentspec.Tool, 0, len(tools))
	for _, t := range tools {
		u, err := url.Parse(t.URL)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(u.Hostname(), ".svc.cluster.local") {
			app := strings.TrimSuffix(strings.Split(u.Hostname(), ".")[0], "-service")
			public, err := resolver.AppURL(app)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", t.URL, err)
			}
			t.URL = strings.TrimRight(public, "/") + u.Path
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the console lists no tools for %s; declare them in %s",
			spec.Name, agentspec.OverridePath)
	}
	return out, nil
}
