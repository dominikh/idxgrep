package chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/fs"
	"honnef.co/go/idxgrep/index"
)

type Conversation struct {
	Protocol        string `json:"protocol"`
	Server          string `json:"server"`
	ChannelOrPerson string `json:"channel_or_person"`
}

type Message struct {
	Conversation

	Time    time.Time
	From    string
	To      []string
	Message string
}

type message struct {
	Conversation

	Time    int      `json:"time"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Message string   `json:"message"`
}

func (m *Message) MarshalJSON() ([]byte, error) {
	msg := message{
		Conversation: m.Conversation,
		Time:         int(m.Time.UnixNano() / int64(time.Millisecond)),
		From:         m.From,
		To:           m.To,
		Message:      m.Message,
	}
	return json.Marshal(msg)
}

func (m *Message) UnmarshalJSON(b []byte) error {
	var msg message
	err := json.Unmarshal(b, &msg)
	if err != nil {
		return err
	}
	m.Conversation = msg.Conversation
	m.Time = time.Unix(0, int64(msg.Time)*int64(time.Millisecond))
	m.From = msg.From
	m.To = msg.To
	m.Message = msg.Message
	return nil
}

func (m Message) String() string {
	return fmt.Sprintf("%s://%s/%s %s <%s> %s",
		m.Protocol,
		m.Server,
		m.ChannelOrPerson,
		m.Time,
		m.From,
		m.Message,
	)
}

type Index struct {
	Client *es.Client
}

func (idx *Index) CreateIndex() error {
	body := `
    {
      "settings": {
        "number_of_shards": 1,
        "number_of_replicas": 0,
        "analysis": {
          "char_filter": {
            "username": {
              "type": "pattern_replace",
              "pattern": "^[+%@]?(.+)$",
              "replacement": "$1"
            }
          },
          "normalizer": {
            "username": {
              "type": "custom",
              "char_filter": ["username"],
              "filter": ["lowercase"]
            }
          }
        }
      },
      "mappings": {
        "_doc": {
          "properties": {
            "protocol": {
              "type": "keyword"
            },
            "server": {
              "type": "keyword"
            },
            "channel_or_person": {
              "type": "keyword"
            },
            "time": {
              "type": "date",
              "format": "epoch_millis"
            },
            "from": {
              "type": "keyword",
              "normalizer": "username",
              "fields": {
                "raw": {
                  "type": "keyword"
                }
              }
            },
            "to": {
              "type": "keyword",
              "normalizer": "username",
              "fields": {
                "raw": {
                  "type": "keyword"
                }
              }
            },
            "message": {
              "type": "text",
              "analyzer": "english"
            }
          }
        }
      }
    }
    `

	req, err := http.NewRequest("PUT", idx.Client.Base+"/"+idx.Client.Index, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	resp, err := idx.Client.Do(req)
	if err != nil {
		if err, ok := err.(es.APIError); ok {
			if err.Err.Type == "resource_already_exists_exception" {
				return nil
			}
		}
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (idx *Index) Search(s es.Search, count int) ([]Message, error) {
	hits, err := idx.Client.Search(s, count)
	if err != nil {
		return nil, err
	}
	out := make([]Message, len(hits))
	for i, hit := range hits {
		if err := json.Unmarshal(hit.Source, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type Weechat struct {
	Client *es.Client
}

func (w *Weechat) CreateIndex() error {
	return (&Index{w.Client}).CreateIndex()
}

var ircToRegexp = regexp.MustCompile(`^([^: ]+): `)

func (w *Weechat) Index(root string) (index.Statistics, error) {
	// TODO(dh): support folder/file layouts other than the one I am
	// using

	root = path.Join(root, "irc")
	f, err := fs.Open(root)
	if err != nil {
		return index.Statistics{}, err
	}
	defer f.Close()
	channels, err := f.Readdirnames(0)
	if err != nil {
		return index.Statistics{}, err
	}
	bi := w.Client.BulkInsert()
	stats := index.Statistics{}
	for _, channel := range channels {
		idx := strings.Index(channel, "#")
		if idx == -1 {
			idx = strings.LastIndex(channel, ".")
			if idx == -1 {
				continue
			}
			idx++
		}

		dir, err := fs.Open(path.Join(root, channel))
		if err != nil {
			log.Println(err)
			continue
		}
		files, err := dir.Readdirnames(0)
		dir.Close()
		if err != nil {
			log.Println(err)
			continue
		}

	fileLoop:
		for _, file := range files {
			conv := Conversation{
				Protocol:        "irc",
				Server:          channel[:idx-1],
				ChannelOrPerson: channel[idx:],
			}

			f, err := fs.Open(path.Join(root, channel, file))
			if err != nil {
				log.Println(err)
				continue
			}

			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				if line == "irc: disconnected from server" {
					continue
				}
				fields := strings.SplitN(line, "\t", 3)
				if len(fields) != 3 {
					// log.Printf("Couldn't parse %q", line)
					// likely not weechat logs
					f.Close()
					stats.Skipped++
					continue fileLoop
				}
				t, err := time.Parse("2006-01-02 15:04:05 ", fields[0])
				if err != nil {
					log.Println(err)
					// likely not weechat logs
					f.Close()
					stats.Skipped++
					continue fileLoop
				}
				from := fields[1]
				switch from {
				case "--", "<--", "-->", "←", "→", "":
					continue
				}
				text := fields[2]
				m := ircToRegexp.FindStringSubmatch(text)
				var to []string
				if m != nil {
					to = []string{m[1]}
				}
				msg := &Message{
					Conversation: conv,
					Time:         t,
					From:         from,
					To:           to,
					Message:      text,
				}
				if err := bi.Index(msg, ""); err != nil {
					bi.Close()
					return index.Statistics{}, err
				}
			}
			stats.Indexed++
			f.Close()
		}
	}
	if err := bi.Close(); err != nil {
		return index.Statistics{}, err
	}
	return stats, nil
}
