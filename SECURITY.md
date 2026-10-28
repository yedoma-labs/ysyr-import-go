# Security Policy

## Supported Versions

Only the latest released version receives security fixes.

## Reporting a Vulnerability

Please do not open a public issue for security problems. Report them privately through
GitHub's Private Vulnerability Reporting:

1. Open the repository on GitHub.
2. Go to the **Security** tab.
3. Click **Report a vulnerability** and describe the issue.

Include the affected version, steps to reproduce, and the expected and actual behavior. You will
get an acknowledgement, and the report stays private until a fix is released.

## Scope

In scope:

- Inputs that bypass the limits `WithMaxBytes`, `WithMaxRows` or `WithMaxFieldLength`, or that
  exhaust memory or CPU beyond what the configured limits allow.
- CSV injection in the output of `WriteErrorReport`: a cell that a spreadsheet program would
  evaluate as a formula without the `'` prefix.
- Messages in `RowError` that carry control characters, or raw input beyond the 100-character
  limit.

Out of scope:

- Storing the accepted rows, and transactions around them. This is the caller's responsibility.
- Validation rules and parsers written by the caller, such as a custom `Column` parse function.
