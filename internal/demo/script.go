// Package demo is a fake tuimessenger backend for recording demos and screenshots: the real client API and store,
// seeded with made-up chats on all four networks, behind a fake network layer that acknowledges what you send. It's
// only reachable through the devdemo build (tuimessenger demo …), never in releases.
package demo

import "time"

// person is a made-up contact. IDs use the reserved fictional ranges the privacy check allows (+1 555 01xx, 1000…).
type person struct {
	id   string
	name string
}

// msg is one seeded message. from is a person's index in the chat, or you.
type msg struct {
	from   int
	text   string
	ago    time.Duration
	image  string // file in the media folder
	video  string
	reply  int // 1-based index of the message this replies to, 0 for none
	reacts map[int]string
}

const you = -1

// chat is one seeded conversation; name is empty for a DM.
type chat struct {
	key       string
	network   string
	name      string
	people    []person
	msgs      []msg
	unreadAt  int      // messages from this index on are unread (len(msgs) for none)
	readByAll bool     // your last message was seen by the others
	replies   []string // what the other side answers to your next messages, in order
}

func chats() []*chat {
	maya := person{"15550100011", "Maya"}
	arjunWA := person{"15550100012", "Arjun"}
	lenaWA := person{"15550100013", "Lena"}
	mom := person{"15550100014", "Mom"}
	arjunTG := person{"10000000021", "Arjun"}
	kai := person{"10000000022", "Kai"}
	noor := person{"10000000023", "Noor"}
	lenaIG := person{"100000000031", "lena.creates"}
	priya := person{"t0000000000-u0000000041", "Priya"}
	sam := person{"t0000000000-u0000000042", "Sam"}
	h := time.Hour
	m := time.Minute
	return []*chat{
		{
			key: "maya", network: "whatsapp", people: []person{maya},
			msgs: []msg{
				{from: you, text: "how was the gig last night?", ago: 3 * h},
				{from: 0, text: "SO good 🎶 they played the whole first album", ago: 2*h + 50*m},
				{from: 0, text: "farmers market haul 🥬", ago: 25 * m, image: "food.jpg"},
				{from: 0, text: "dinner at 8? I booked a table near the lake 🍝", ago: 4 * m},
			},
			unreadAt: 2,
			replies:  []string{"perfect, see you there!", "😂 yes", "ok ok"},
		},
		{
			key: "trek", network: "whatsapp", name: "Weekend Trek 🏔️", people: []person{arjunWA, lenaWA, maya},
			msgs: []msg{
				{from: 0, text: "trail opens at 6, who's driving?", ago: 26 * h},
				{from: 1, text: "I can take 3 people 🚗", ago: 25 * h},
				{from: you, text: "count me in", ago: 25 * h},
				{from: 0, text: "view from the top last year", ago: 3 * h, image: "mountains.jpg", reacts: map[int]string{1: "🔥", 2: "❤️"}},
				{from: 2, text: "change of plan: rain after 2pm, let's meet at 5:30 instead ☔", ago: 50 * m},
				{from: 1, text: "don't forget water and snacks", ago: 30 * m, reply: 1},
				{from: 0, text: "can you bring the first aid kit? 🩹", ago: 12 * m},
			},
			unreadAt: 4,
			replies:  []string{"legend 🙌", "see you at 5:30!"},
		},
		{
			key: "mom", network: "whatsapp", people: []person{mom},
			msgs: []msg{
				{from: 0, text: "did you eat? 🍲", ago: 5 * h},
				{from: you, text: "yes ma, dal and rice", ago: 4*h + 50*m},
				{from: 0, text: "good. call me on sunday ❤️", ago: 4*h + 45*m},
			},
			unreadAt: 3, readByAll: true,
			replies: []string{"❤️"},
		},
		{
			key: "arjun", network: "telegram", people: []person{arjunTG},
			msgs: []msg{
				{from: 0, text: "did you see the new Go release?", ago: 6 * h},
				{from: you, text: "yes! iterators everywhere now 😄", ago: 5*h + 58*m},
				{from: 0, text: "rewriting my CLI this weekend, wish me luck", ago: 5*h + 40*m},
			},
			unreadAt: 3, readByAll: true,
			replies: []string{"haha thanks", "will send you the repo"},
		},
		{
			key: "terminal", network: "telegram", name: "Terminal Nerds", people: []person{kai, noor, arjunTG},
			msgs: []msg{
				{from: 0, text: "anyone tried images inside the terminal yet?", ago: 9 * h},
				{from: 1, text: "kitty graphics in ghostty are wild", ago: 8*h + 30*m},
				{from: 2, text: "I read all my chats in a TUI now, no going back", ago: 50 * m, reacts: map[int]string{0: "😂", 1: "👀"}},
			},
			unreadAt: 2,
			replies:  []string{"which one?", "screenshot or it didn't happen"},
		},
		{
			key: "lena", network: "instagram", people: []person{lenaIG},
			msgs: []msg{
				{from: you, text: "your new reel is so good", ago: 7 * h, reacts: map[int]string{0: "🥹"}},
				{from: 0, text: "this bunny is literally you on mondays 😂", ago: 12 * m, video: "bunny.mp4"},
			},
			unreadAt: 1,
			replies:  []string{"HAHA", "admit it 😂"},
		},
		{
			key: "design", network: "slack", name: "#design", people: []person{priya, sam},
			msgs: []msg{
				{from: 0, text: "new onboarding mockups are up 👀", ago: 2 * h, image: "desk.jpg", reacts: map[int]string{1: "🎉"}},
				{from: 1, text: "love the colors, ship it", ago: h + 40*m, reply: 1},
			},
			unreadAt: 1,
			replies:  []string{"thanks!", "merging after lunch"},
		},
		{
			key: "priya", network: "slack", people: []person{priya},
			msgs: []msg{
				{from: 0, text: "can you review my PR before standup?", ago: 15 * m},
			},
			unreadAt: 0,
			replies:  []string{"thank you 🙏", "fixed, pushed again"},
		},
	}
}

// files the script uses; scripts/demo-media.sh downloads them.
var mediaFiles = map[string]string{
	"food.jpg":      "image/jpeg",
	"mountains.jpg": "image/jpeg",
	"desk.jpg":      "image/jpeg",
	"bunny.mp4":     "video/mp4",
}
