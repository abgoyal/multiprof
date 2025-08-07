# multiprof

## A Tool-Agnostic Context Switcher for the Command Line

***

multiprof lets you manage multiple accounts and configurations for any CLI tool
by sandboxing your home directory based on your current location (cwd).

It prevents credential conflicts and accidental operations by ensuring that when
you are working in a specific project directory, your tools (like `aws`,
`gcloud`, `gh`, `kubectl`, etc.) only use the configuration and credentials
associated with that project.


***
## The Motivation

If you're a consultant, a freelancer, or work on multiple projects, you've likely
faced this problem:

- You run `aws s3 ls` and see buckets from a personal project while working for a client.
- You run `gcloud auth login` and accidentally overwrite your work credentials with your personal ones.
- You run `gh pr create` and create a pull request as the wrong GitHub user.

Most CLI tools store their configuration (credentials, settings, cache) in your
single home directory (`~`). While some tools have built-in "profile" features,
they are often inconsistent, require manual flag-setting (`--profile=...`), and
don't isolate other data like package caches or history files.

multiprof solves this problem with a simple, universal, and powerful principle:
it changes the `$HOME` environment variable itself. This forces any tool you run
through it into a clean, sandboxed environment unique to your current context.


***
## How It Works

The system is built on a clear and simple foundation. Understanding these three
components is key to using multiprof effectively.

### The Core Components

1.  **The `multiprof` Tool:** The command-line program you use to manage the
    system. You run `multiprof init` to set it up and `multiprof add-rule` to
    define your contexts.

2.  **The Wrapper Directory:** A dedicated directory (e.g., `~/.local/bin/multiprof`)
    that holds all your command wrappers. 

3.  **The Wrappers:** These are simply symlinks to the `multiprof` executable,
    residing in the Wrapper Directory. For example, `aws_w` or a seamless `aws`
    are both symlinks pointing to the `multiprof` binary.

### The Execution Flow

When you run a **Wrapper** like `aws_w`:
1.  Your shell searches the `$PATH` and finds `aws_w` in the **Wrapper Directory**.
2.  The `multiprof` binary executes.
3.  It detects it was run as `aws_w`, not `multiprof`, so it enters **wrapper mode**.
4.  It checks your Current Working Directory (CWD).
5.  It reads your config file (`~/.config/multiprof/config.toml`) and finds the
    first **Rule** where the CWD matches the rule's `pattern`.
6.  It sets the `$HOME` environment variable to the `home` directory specified in that Rule.
7.  It then temporarily modifies the `$PATH` to hide the Wrapper Directory, ensuring it
    finds the *real* `aws` executable.
8.  Finally, it replaces its own process with the real `aws` command, which now runs
    entirely within the sandboxed `$HOME` you defined.


***
## Understanding Glob Patterns

The `pattern` field in your rules uses globbing, which is a simpler form of
pattern matching than regular expressions. The first rule in your config file that
matches the current directory wins.

-   `*` : Matches any sequence of characters, but **not** a path separator (`/`).
-   `**`: Matches any sequence of characters, **including** path separators. This is the key to matching all subdirectories.
-   `?` : Matches any single character.
-   `{a,b}`: Matches either `a` or `b`.
-   `[]`: Matches any character within the brackets.

### Common Examples

**1. Match a directory and all its subdirectories (most common case):**
This rule will activate when you are in `~/work/client-a` or any directory below it.

```toml
pattern = "~/work/client-a/**"
````

**2. Match only the immediate children of a directory:**
This rule will match `~/projects/proj1` and `~/projects/proj2`, but it will **not** match `~/projects/proj1/src`.

```toml
pattern = "~/projects/*"
```

-----

## How Tab Completion Works (And the Suffix Trade-Off)

Getting tab completion right is essential. multiprof supports two methods, each with a distinct trade-off regarding your `$PATH` setup.

### Method 1: Seamless Experience (Empty Suffix)

This method is the most transparent but has a strict requirement.

  - **Setup:** You set `suffix = ""` in your config and create a wrapper named `aws`.
  - **How it works:** When you type `aws <TAB>`, your shell looks for completions for `aws` and finds the original system completions automatically.
  - **The Requirement:** For this to work, your **Wrapper Directory must be the very first entry in your `$PATH`**. This ensures the shell finds your `multiprof` wrapper named `aws` before it finds the system's real `/usr/bin/aws`.

### Method 2: Flexible Path (Suffixed Wrappers)

This method is more flexible if you cannot or do not want to modify the beginning of your `$PATH`.

  - **Setup:** You use a non-empty suffix, like `_w`, creating wrappers named `aws_w`.
  - **How it works:** The `eval "$(multiprof generate-completions)"` command in your shell profile teaches your shell how to provide completions for `aws_w` by using the settings from the original `aws`.
  - **The Advantage:** Because `aws_w` is a unique command name, it doesn't matter where the **Wrapper Directory** is in your `$PATH` (as long as it's included somewhere). There's no risk of conflict with the original tool.

-----

## Real-World Walkthroughs

Theory is good, but seeing how `multiprof` solves real problems makes it click.
Here are two common scenarios.

### Scenario 1: The Consultant (Flexible Path with Suffixed Wrappers)

This is the most straightforward approach and doesn't require strict `$PATH` ordering. Imagine you're a consultant juggling two major clients, "MegaCorp" and "StartupX".

**Step 1: One-Time Initialization**
First, run the setup wizard.

```sh
multiprof init
```

Follow the on-screen instructions to add the **Wrapper Directory** to your `$PATH` and to set up completions.

**Step 2: Create Your Context Directories**

```sh
mkdir -p ~/clients/megacorp
mkdir -p ~/clients/startupx
```

**Step 3: Define Your Rules**

```sh
multiprof add-rule --pattern '~/clients/megacorp/**' --home '~/clients/megacorp'
multiprof add-rule --pattern '~/clients/startupx/**' --home '~/clients/startupx'
```

**Step 4: Create Wrappers for Your Tools**
We will use the default `_w` suffix.

```sh
multiprof add-wrapper aws
multiprof add-wrapper gcloud
```

**Step 5: Configure Each Context**
`cd` into each context directory and configure your tools using the wrappers.
*Configure MegaCorp:*

```sh
cd ~/clients/megacorp
aws_w configure  # Enter credentials for MegaCorp
gcloud_w auth login # Log in with MegaCorp Google account
```

*Configure StartupX:*

```sh
cd ~/clients/startupx
aws_w configure  # Enter credentials for StartupX
```

**Step 6: It Just Works\!**

```sh
cd ~/clients/megacorp/projects/analytics-pipeline
aws_w s3 ls # This lists buckets for MegaCorp

cd ~/clients/startupx/projects/api-service
aws_w s3 ls # This lists buckets for StartupX
```

### Scenario 2: The Power User (Seamless Empty Suffix)

This method offers a completely transparent experience but **requires** ensuring the Wrapper Directory is first in your `$PATH`, as instructed by `multiprof init`.

**Step 1: `init` and Edit Config**
First, run `multiprof init` and set up your shell, making sure the `export PATH` line is correct. Then, edit `~/.config/multiprof/config.toml` to use an empty suffix:

```toml
suffix = ""
```

**Step 2: Create Context Directories and Rules**

```sh
mkdir -p ~/work
mkdir -p ~/personal
multiprof add-rule --pattern '~/work/**' --home '~/work'
multiprof add-rule --pattern '~/personal/**' --home '~/personal'
```

**Step 3: Create Suffix-less Wrappers**

```sh
multiprof add-wrapper aws
multiprof add-wrapper gh
```

This creates wrappers named `aws` and `gh`.

**Step 4: Configure and Use**
The experience is now completely seamless, and tab-completion works automatically.

```sh
cd ~/work
aws configure   # Note: not aws_w
gh auth login   # Login with your work GitHub account

cd ~/personal
gh auth login   # Login with your personal GitHub account
```

-----

## Installation

Download a pre-compiled binary from the Releases page or build it from source.

**To Build from Source:**

1.  Ensure you have Go installed (version 1.18+).
2.  Have the necessary files in one directory: `multiprof.go`, `help.txt`, `default.toml`.
3.  Run the build command: `go build -o multiprof .`
4.  Move the binary to a location in your PATH: `mv ./multiprof ~/.local/bin/`

-----

## Command Reference

  - `init`: Runs the one-time setup wizard. It's safe to run again to see instructions.
  - `add-rule --pattern <p> --home <h>`: Adds a context Rule to your config.
  - `add-wrapper <command>`: Creates a new Wrapper in your Wrapper Directory.
  - `list`: Lists all configured Rules in their order of priority.
  - `generate-completions`: Generates shell completion code for suffixed Wrappers.
  - `help`: Shows the main help text.

-----

## Hacking on `multiprof`

The project consists of a single `multiprof.go` file and two embedded text files,
`help.txt` and `default.toml`.

  - **Build:** `go build -o multiprof .`
  - **Core Logic:** The main logic is split between `runWrapper()` (for when it acts as
    a wrapper) and `runManager()` (which dispatches management commands). The `main()`
    function determines which mode to enter based on the executable name.
  - **Dependencies:** `github.com/BurntSushi/toml` and `github.com/gobwas/glob`.

