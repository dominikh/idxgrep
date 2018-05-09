package chat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"honnef.co/go/idxgrep/es"
	"honnef.co/go/idxgrep/fs"
	"honnef.co/go/idxgrep/index"
)

type Discord struct {
	Client *es.Client
}

func (w *Discord) CreateIndex() error {
	return (&Index{w.Client}).CreateIndex()
}

var discordUserMention = regexp.MustCompile(`<@(\d+)>`)

func (d *Discord) Index(path string) (index.Statistics, error) {
	type user struct {
		Name string `json:"name"`
	}
	type server struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	type channel struct {
		Server int    `json:"server"`
		Name   string `json:"name"`
	}
	type meta struct {
		Users     map[string]user    `json:"users"`
		UserIndex []string           `json:"userindex"`
		Servers   []server           `json:"servers"`
		Channels  map[string]channel `json:"channels"`
	}
	type message struct {
		User      int    `json:"u"`
		Timestamp int64  `json:"t"`
		Message   string `json:"m"`
	}
	var log struct {
		Meta meta                          `json:"meta"`
		Data map[string]map[string]message `json:"data"`
	}

	f, err := fs.Open(path)
	if err != nil {
		return index.Statistics{}, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&log); err != nil {
		return index.Statistics{}, err
	}

	bi := d.Client.BulkInsert()
	for chid, msgs := range log.Data {
		ch := log.Meta.Channels[chid]
		srv := log.Meta.Servers[ch.Server]
		var chName string
		switch srv.Type {
		case "SERVER":
			chName = "#" + ch.Name
		case "DM":
			chName = ch.Name
		default:
			return index.Statistics{}, fmt.Errorf("unknown server type %q", srv.Type)
		}
		conv := Conversation{
			Protocol:        "discord",
			Server:          srv.Name,
			ChannelOrPerson: chName,
		}
		for mid, msg := range msgs {
			user := log.Meta.Users[log.Meta.UserIndex[msg.User]].Name
			ts := time.Unix(0, msg.Timestamp*int64(time.Millisecond))

			var mentions []string
			msg.Message = discordUserMention.ReplaceAllStringFunc(msg.Message, func(match string) string {
				id := match[2 : len(match)-1]
				user := log.Meta.Users[id]

				if srv.Type == "SERVER" {
					mentions = append(mentions, user.Name)
				}
				return "@" + user.Name
			})
			if srv.Type == "DM" {
				// XXX figure out our own name
				mentions = []string{}
			}
			m := &Message{
				Conversation: conv,
				Time:         ts,
				From:         user,
				To:           mentions,
				Message:      msg.Message,
			}
			bi.Index(m, fmt.Sprintf("discord-%s-%s-%s", srv.Name, chid, mid))
		}
	}
	if err := bi.Close(); err != nil {
		return index.Statistics{}, err
	}
	return index.Statistics{Indexed: 1}, nil
}
