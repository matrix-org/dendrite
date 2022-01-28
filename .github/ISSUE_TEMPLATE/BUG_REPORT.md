---
name: Bug report
about: Create a report to help us improve

---

<!--
All bug reports must provide the following background information
Text between <!-- and --​> marks will be invisible in the report.
-->

### Background information
<!-- Please include versions of all software when known e.g database versions, docker versions, client versions -->
- **Dendrite version or git SHA**: 
- **Monolith or Polylith?**: 
- **SQLite3 or Postgres?**: 
- **Running in Docker?**: 
- **`go version`**:
- **Client used (if applicable)**:


### Description

 - **What** is the problem:
 - **Who** is affected:
 - **How** is this bug manifesting:
 - **When** did this first appear:

<!--
Examples of good descriptions:
- What: "I cannot log in, getting HTTP 500 responses"
- Who: "Clients on my server"
- How: "Errors in the logs saying 500 internal server error"
- When: "After upgrading to 0.3.0"

- What: "Dendrite ran out of memory"
- Who: "Server admin"
- How: "Lots of logs about device change updates"
- When: "After my server joined Matrix HQ"


Examples of bad descriptions:
- What: "Can't send messages"  - This is bad because it isn't specfic enough. Which endpoint isn't working and what is the response code? Does the message send but encryption fail?
- Who: "Me" - Who are you? Running the server or a user on a Dendrite server?
- How: "Can't send messages" - Same as "What".
- When: "1 day ago" - It's impossible to know what changed 1 day ago without further input.
-->

### Steps to reproduce
<!-- Please try reproducing this bug before submitting it. Issues which cannot be reproduced risk being closed. -->

- list the steps
- that reproduce the bug
- using hyphens as bullet points

<!--
Describe how what happens differs from what you expected.

If you can identify any relevant log snippets from server logs, please include
those (please be careful to remove any personal or private data). Please surround them with
``` (three backticks, on a line on their own), so that they are formatted legibly.

Alternatively, please send logs to @kegan:matrix.org or @neilalexander:matrix.org
with a link to the respective Github issue, thanks!
-->
