# idxgrep – indexed grep

Idxgrep is a desktop search engine for /text documents/, using
Elasticsearch for indexing, and supporting regular expressions. It can
be used as a very fast, system-wide alternative to grep.

Text documents can be plain files, files in archives, git commits, or
more abstract concepts.

Idxgrep is based on Russ Cox's
[Regular Expression Matching with a Trigram Index](https://swtch.com/~rsc/regexp/regexp4.html).

## Status

Idxgrep is in its early prototyping stage. Features are missing, usage
is clunky, the index format will change.

## Installation

```
go get -u honnef.co/go/idxgrep/cmd/...
```

## Features

### File type support

Idxgrep tries to transparently index files within files, such as files
in ZIP archives or revisions in version control. When possible, it
does this recursively, such as a ZIP inside another ZIP, or a zipped
repository (though this depends on the specific file types).

Currently supported file types include:

- ZIP
- tar

### Ignoring files

Files can be omitted from the index based on the following filters:

- Maximum file size
- Not being binary data
- Not being special files (such as named pipes or block devices)
- Not having certain names (for example `__MACOSX`)

### Planned features

These features aren't implemented yet but will be in the future:

- Support for non-local files. http://, git://, possibly others?
- Indexing revisions in version control systems (git, possibly others?)
- A daemon that watches for file changes and automatically updates the index

## Usage

There are three basic commands for using idxgrep: `idxgrep`, `idxadd`
and `idxrm`. `idxadd` adds a folder to the index, `idxrm` removes a
folder from the index, and `idxgrep` searches in the index.

## Configuration

Idxgrep looks for a configuration file named `idxgrep.conf` in the following places:

| OS      | Path                                                          |
|---------|---------------------------------------------------------------|
| Windows | `%APPDATA%\idxgrep`                                           |
| Linux   | `$XDG_CONFIG_HOME/idxgrep` (default: `$HOME/.config/idxgrep`) |
| macOS   | `$HOME/Library/Application Support/idxgrep`                   |

If no configuration file can be found, default settings will be used.
These match the values in `example.conf`.

### Elasticsearch

Idxgrep uses Elasticsearch for indexing files and relies on specific
mappings. Idxgrep will create an index with the required mappings if
it doesn't exist yet. You are, however, free to create it yourself,
for example if you wish to configure the number of shards and
replicas.

The automatically created index is as follows:

```
{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0,
    "analysis": {
      "tokenizer": {
        "trigram": {
          "type": "ngram",
          "min_gram": 3,
          "max_gram": 3
        },
        "path": {
          "type": "path_hierarchy",
          "delimiter": "/"
        }
      },
      "char_filter": {
        "nul_to_slash": {
          "type": "pattern_replace",
          "pattern": "\u0000",
          "replacement": ""
        }
      },
      "analyzer": {
        "trigram": {
          "type": "custom",
          "tokenizer": "trigram"
        },
        "path": {
          "type": "custom",
          "tokenizer": "path",
          "char_filter": ["nul_to_slash"]
        }
      }
    }
  },
  "mappings": {
    "_doc": {
      "_source": {
        "enabled": false
      },
      "_all": {
        "enabled": false
      },
      "properties": {
        "name": {
          "type": "keyword",
          "store": true
        },
        "path": {
          "type": "text",
          "analyzer": "path",
          "store": true
        },
        "data": {
          "type": "text",
          "analyzer": "trigram",
          "index_options": "docs"
        }
      }
    }
  }
}
```
