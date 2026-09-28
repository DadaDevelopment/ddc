# ddc

The Dada Cloud CLI. One binary, two jobs: deploy an app, and run the agent a
repo describes.

```
usage: ddc <command>

commands:
  login            sign in via your browser (device code flow)
  deploy [dir]     package and deploy dir (default: current directory)
  agent <action>   run the agent described by this repo's .dada/agent.json
```

This repo is public on purpose: it is the only part of Dada Cloud users install
on their own machines, so the console can stay private without breaking
`install.sh`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/DadaDevelopment/ddc/main/install.sh | sh
```

Go 1.25, no third-party dependencies. `ddc agent` additionally needs Docker.

## ddc agent

There is no manifest to write. An agent repo is recognised by its layout, and
`ddc` is already signed in, so nothing in the repo has to name a project, an
environment or an agent:

```
agents/<name>/
  core.md              prompt, the file the prod runtime serves
  domains/*.md         skills, served through load_skill
  evals/suites/*.yaml  deterministic eval scenarios
  judge/*.yaml         llm-as-judge rules and prompts
```

Everything else has a default. `.dada/agent.json` is optional and holds only
what genuinely belongs to the repo rather than to a person - non-default tools,
a model override, or an image pinned away from the platform default:

```json
{
  "agents": {
    "tg-exchange-support": {
      "model": { "name": "glm-5.3-flash", "api_key_env": "MODEL_API_KEY" },
      "tools": [{ "url": "https://tools.example/mcp" }]
    }
  }
}
```

Only the *name* of the key variable is stored. Secrets come from the
environment or `.ddc/.env` (gitignore `.ddc/`).

A tool header refers to its secret the same way, as `${VAR}`:
`"headers": {"Authorization": "Bearer ${TOOLS_TOKEN}"}`. `ddc agent up` fills
it from your environment or `.ddc/.env` and refuses to start when it is unset
(a local agent without its tool credentials answers toolless); `ddc agent
deploy` sends the header as written and the platform fills it from the
deployed agent's own env var, so a deploy never drops the credential.

Where the agent is deployed is not in the repo either: it is asked once and
remembered per directory in your own config, exactly as `ddc deploy` remembers
an app's project. Two people can run the same repo against different
environments without editing a committed file.

```bash
cd my-agent-repo
ddc agent spec       # what the manifest resolves to - run this when a layout looks wrong
ddc agent up         # start the agent from the spec
ddc agent smoke      # one real A2A turn, exit 1 on silence
ddc agent eval       # run every eval suite against it (--suite <name> for one)
ddc agent invoke --text "привет"   # one message to the deployed agent, prints the reply
ddc agent deploy --project <name> --env <name>   # asked once, then remembered
ddc agent logs -f
ddc agent down
```

### Deploy

`ddc agent deploy` reads the repo and calls the console's API. The dependency
points that way on purpose: production does not read files out of your
repository, so the layout above is ddc's contract rather than the platform's,
and the UI is skipped entirely.

CI runs the same two commands a developer runs - `ddc agent eval` and
`ddc agent deploy` - so a pipeline is never a separate code path. CI has no
browser, so it authenticates with `DDC_TOKEN`, or with `DDC_SERVICE_CLIENT_ID`
plus `DDC_SERVICE_CLIENT_SECRET` for the client-credentials grant.

A deploy is finished when its operation reaches `Committed`: the gitops agent
ends an agent write there and nothing advances that row afterwards.

Flags: `--repo`, `--agent` (when the manifest declares several), `--prompt` to
try another prompt without touching the spec, `--mcp URL` (repeatable) to
override the tool servers, `--port` (default 18081).

Tool priority: `--mcp`, then the tools in `.dada/agent.json`, then the console -
ddc asks the API which tools the agent runs with in its remembered environment
and rewrites in-cluster URLs to their public ones.

The agent runs in the production kagent image: the patched build published by
CI from [DadaDevelopment/kagent](https://github.com/DadaDevelopment/kagent),
which is the default so a repo pins nothing. What runs locally is what runs in
production, tracing patch included. The rendered config is copied
into the container rather than bind-mounted, so a remote or docker-in-docker
daemon works the same. `up` returns only once the agent answers `/health`.

## Exit codes

Every `ddc agent` action follows one contract, so a pipeline can tell a failing
agent from a broken invocation:

```
0   pass
1   fail       an eval gate missed, a smoke turn was silent, invoke got no reply
2   config     unknown flag or action, missing suite or runner, no local agent,
               or the eval runner itself exited 2 (argparse, bad arguments)
```

## Eval reports

Evals are report-only: nothing is sent to Langfuse, because eval traffic is
synthetic and must not spend the observability quota. `ddc agent eval` runs
`scripts/eval_run.py` from the agent repo once per suite in
`agents/<name>/evals/suites/*.yaml` and writes each into its own directory:

```
eval-artifacts/<suite>/
  report.json     the whole run: pass_rate, passed/total, lost turns, per-scenario
                  results and top failures, label, transport, git sha and CI run url
  summary.md      the same as a markdown table; also appended to
                  $GITHUB_STEP_SUMMARY when set
  history.jsonl   one line per finished run (report.json without scenarios),
                  appended, so a kept directory shows the trend over time
  transcripts/    the raw turns, re-scorable with --from-transcripts
```

With `--suite <name>` a single suite runs and the runner's own `--output-dir`
is used as is. Runner flags go through `DDC_EVAL_ARGS`, for example
`DDC_EVAL_ARGS="--label ci --fail-below 0.8 --repeats 3"`; an `--output-dir`
there becomes the parent of the per-suite directories. The exit code is the
worst suite: 2 if any runner exited 2, else 1 if any missed `--fail-below`.

The eval runner, its cases and the judge live in the agent repo, next to the
agent they belong to.
