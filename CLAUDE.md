### Project Purpose

- An E2E framework to allow testing celestia ecosystem projects in docker environments.
  - specifically to these repos
    - https://github.com/celestiaorg/celestia-app
    - https://github.com/celestiaorg/celestia-node
    - https://github.com/rollkit/rollkit

### Project Structure

- `framework/docker` Provide implementations to deploy components in docker.
- `framework/testutil` Contains utility functions in subpackages.
  - these functions should depend on small interfaces defined in the same package to avoid dependency cycles.
- `framework/types` Contains the main set of interfaces that consuming libraries will import.

### Commands

- `make lint-fix` lint the project
- `make test-short` runs tests in short mode, this does not deploy any docker resources.
- `make test` run tests for the project (this can take time as it deploys docker resources)

### Do Not Section

- don't use any emojis.
- avoid excessive comments, only include comments when the intent is not clear, do not add comments saying what the code does.

### Other

- If any configuration changes such as adding new environment variables, make sure their usage is properly outlined in README.md
