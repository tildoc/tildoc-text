# tildoc-text

Text extraction for [tildoc](https://github.com/tildoc/tildoc): plain-text files and text inside PDFs, on originals and on variants other plugins make. It talks to tildoc only through the plugin REST API and needs no database and no master key.

| Variable | Meaning |
|---|---|
| `PLUGIN_LISTEN` | Where it takes jobs, default `:8000` |
| `TILDOC_URL` | Address of the tildoc server, e.g. `http://server:8080` |
| `PLUGIN_CONCURRENCY` | Files at a time, default 4 |

Licence: GNU AGPL-3.0-only, like tildoc.

The program holds no credential. tildoc hands it each job with the file and a token that can only deliver the result of that job.
