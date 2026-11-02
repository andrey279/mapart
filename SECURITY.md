# Security

Report suspected vulnerabilities privately to the repository owner. If private
vulnerability reporting is enabled, use **Security → Report a vulnerability**.
Otherwise, contact the owner through the contact information on their GitHub
profile. Avoid posting credentials or sensitive image data in public issues.

Include the affected revision or release, operating system, reproduction steps
and expected impact. Security fixes are documented in release notes; this
project does not promise a response time or a version support schedule.

The application processes local images and can download images supplied through
`-links`. Use URLs you trust. Downloads are decoded as images, not executed as
programs. Large images and large exports can use substantial memory and disk
space. Exports may replace existing files at the selected destination.

Use current Go dependencies and run the application with ordinary user
permissions. In containers, mount only required input and output directories.
See `go.mod` for the current dependency list and `licenses/` for notices.
