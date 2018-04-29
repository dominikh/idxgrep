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
      "analyzer": {
        "trigram": {
          "type": "custom",
          "tokenizer": "trigram"
        },
       "path": {
         "type": "custom",
         "tokenizer": "path"
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
