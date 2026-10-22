I've learned a lot by looking at other people's code so
maybe someone can learn something from looking at mine.

= = = = = = = =

Gator is a desktop personal agent for ongoing work.

Open the Gator app to start a conversation, continue an 
ongoing goal, review follow-up results, and manage saved 
knowledge. Gator can research selected sources, analyze 
data, create documents, and prepare actions through selected
tools. 

External changes still require explicit approval.

To run from source, install Go 1.25.13+ and Node.js 22.12+,
then run these commands from the repository root:

  make run       Build and open Gator
  make build     Build the private runtime and frontend
  make package   Build an unpacked app in desktop/release/
  make dist      Build installers in desktop/release/
  make help      List development commands

Desktop dependencies install automatically when needed.
See docs/DEVELOPMENT.md for setup without Make.

= = = = = = = =

🐊
