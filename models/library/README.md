# NetLogo Models Library

This directory is a snapshot of the [NetLogo Models Library](https://github.com/NetLogo/models), the official models repository from Uri Wilensky and the Center for Connected Learning and Computer-Based Modeling (CCL) at Northwestern University.

- Source: https://github.com/NetLogo/models
- Commit: `bf0626a89358b48494066862843a91f36f2556cf` (2026-10-20)
- Upstream readme: https://github.com/NetLogo/models/blob/bf0626a89358b48494066862843a91f36f2556cf/README.md

The folder layout matches the upstream library: Sample Models, Code Examples, Curricular Models, IABM Textbook, HubNet Activities, 3D, and Alternative Visualizations.

## What was copied

| Kind | Count |
| --- | --- |
| `.nlogox` models | 578 |
| `.nlogox3d` models | 37 |
| `.nls` include | 1 |
| `.nlm` files | 2 |
| Companion data (CSV, text, GIS, images the models reference, and similar small files) | 84 |

This snapshot of `main` has no `.nlogo` files. The classic text model `Robby the Robot.nlogo` shipped with GoNetLogo stays at `models/Robby the Robot.nlogo` (CC0, Models Library tag 6.4.0). The library copy is `Sample Models/Computer Science/Robby the Robot.nlogox`.

Preview images that share a model file's base name were left out. They are library thumbnails, not data the model reads. Upstream tests, Scala sources, and build files were left out. No remaining companion file was larger than 2 MiB.

Each model's Info tab and code comments are unchanged.

## Licenses

The models are not covered by the GPL that applies to GoNetLogo. Upstream states:

> The models in this repository are provided under a variety of licenses.
>
> Some models are public domain, some models are open source, some models are [CC BY-NC-SA](http://creativecommons.org/licenses/by-nc-sa/3.0/) (free for noncommercial distribution and use).
>
> See each model's Info tab for details.

CC BY-NC-SA 3.0: https://creativecommons.org/licenses/by-nc-sa/3.0/

Please cite models and NetLogo the way each model's Info tab asks. For NetLogo itself, the usual citation is:

> Wilensky, U. (1999). NetLogo. http://ccl.northwestern.edu/netlogo/. Center for Connected Learning and Computer-Based Modeling, Northwestern University, Evanston, IL.

## Opening the library

GoNetLogo's Open command starts in this directory. `GONETLOGO_MODELS_DIR` overrides that path. `MODELS_DIR` is the server's read root and defaults to the parent `models/` directory.
