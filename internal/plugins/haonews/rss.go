package newsplugin

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	AtomNS  string     `xml:"xmlns:atom,attr,omitempty"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	AtomLink      *rssAtom  `xml:"atom:link,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssAtom struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate,omitempty"`
	Description string  `xml:"description,omitempty"`
	Author      string  `xml:"author,omitempty"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr,omitempty"`
	Value       string `xml:",chardata"`
}

func TopicRSSPath(name string) string {
	name = canonicalTopic(name)
	if name == "" {
		return ""
	}
	return TopicPath(name) + "/rss"
}

func WriteTopicRSS(w http.ResponseWriter, r *http.Request, project, topic string, posts []Post) error {
	payload, last, err := TopicRSSBytes(r, project, topic, posts)
	if err != nil {
		return err
	}
	if !last.IsZero() {
		w.Header().Set("Last-Modified", last.UTC().Format(http.TimeFormat))
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, err = w.Write(payload)
	return err
}

func TopicRSSBytes(r *http.Request, project, topic string, posts []Post) ([]byte, time.Time, error) {
	topic = canonicalTopic(topic)
	feedURL := absoluteURL(r, TopicRSSPath(topic))
	channelURL := absoluteURL(r, TopicPath(topic))
	items := make([]rssItem, 0, len(posts))
	var last time.Time
	for _, post := range posts {
		itemURL := absoluteURL(r, "/posts/"+strings.TrimSpace(post.InfoHash))
		description := strings.TrimSpace(post.Summary)
		if description == "" {
			description = strings.TrimSpace(post.Message.Title)
		}
		if site := strings.TrimSpace(post.SourceSiteName); site != "" {
			if description == "" {
				description = "Source: " + site
			} else if !strings.Contains(description, site) {
				description += " Source: " + site
			}
		}
		pub := ""
		if !post.CreatedAt.IsZero() {
			pub = post.CreatedAt.UTC().Format(time.RFC1123Z)
			if post.CreatedAt.After(last) {
				last = post.CreatedAt
			}
		}
		items = append(items, rssItem{
			Title:       strings.TrimSpace(post.Message.Title),
			Link:        itemURL,
			GUID:        rssGUID{IsPermaLink: "false", Value: post.InfoHash},
			PubDate:     pub,
			Description: description,
			Author:      strings.TrimSpace(post.Message.Author),
		})
	}
	feed := rssFeed{
		Version: "2.0",
		AtomNS:  "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:       project + " Topic: " + topic,
			Link:        channelURL,
			Description: "RSS feed for topic " + topic,
			Language:    "zh-CN",
			Items:       items,
			AtomLink: &rssAtom{
				Href: feedURL,
				Rel:  "self",
				Type: "application/rss+xml",
			},
		},
	}
	if !last.IsZero() {
		feed.Channel.LastBuildDate = last.UTC().Format(time.RFC1123Z)
	}
	payload, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, time.Time{}, err
	}
	var out bytes.Buffer
	out.WriteString(xml.Header)
	out.Write(payload)
	return out.Bytes(), last, nil
}

func absoluteURL(r *http.Request, path string) string {
	if r == nil {
		return path
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	host := strings.TrimSpace(r.Host)
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		return path
	}
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}
	return u.String()
}
