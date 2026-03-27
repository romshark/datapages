// Package list provides a domain model of a simple todo list.
package list

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Item represents a single to-do item.
type Item struct {
	ID          string
	Title       string
	Description string
	Done        bool
	CreatedAt   time.Time
	DueAt       time.Time
}

// List is a thread-safe todo list.
type List struct {
	lock   sync.RWMutex
	todos  []*Item
	nextID atomic.Int64
}

func (l *List) AddItem(title, description string, dueAt time.Time) {
	id := fmt.Sprintf("%x", l.nextID.Add(1))
	l.lock.Lock()
	defer l.lock.Unlock()

	l.todos = append(l.todos, &Item{
		ID:          id,
		Title:       title,
		Description: description,
		CreatedAt:   time.Now(),
		DueAt:       dueAt,
	})
}

func (l *List) ToggleItem(id string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, t := range l.todos {
		if t.ID == id {
			t.Done = !t.Done
			return true
		}
	}
	return false
}

func (l *List) UpdateItem(
	id, title, description string, done bool, dueAt time.Time,
) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, t := range l.todos {
		if t.ID == id {
			t.Title = title
			t.Description = description
			t.Done = done
			t.DueAt = dueAt
			return true
		}
	}
	return false
}

func (l *List) DeleteItem(id string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	for i, t := range l.todos {
		if t.ID == id {
			l.todos = append(l.todos[:i], l.todos[i+1:]...)
			return true
		}
	}
	return false
}

func (l *List) GetItem(id string) (Item, bool) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	for _, t := range l.todos {
		if t.ID == id {
			return *t, true
		}
	}
	return Item{}, false
}

type ViewParameters struct {
	Search string
	Filter string // "all", "done", "pending"
	Sort   string // "alpha", "created", "due"
	ItemID string // only set for PageItem streams
}

func (l *List) GetItems(vp ViewParameters) []Item {
	l.lock.RLock()
	defer l.lock.RUnlock()

	filter, sortMode, search := "all", "created", ""
	if vp != (ViewParameters{}) {
		filter, sortMode, search = vp.Filter, vp.Sort, vp.Search
	}

	var result []Item
	searchLower := strings.ToLower(search)
	for _, t := range l.todos {
		switch filter {
		case "done":
			if !t.Done {
				continue
			}
		case "pending":
			if t.Done {
				continue
			}
		}
		if search != "" &&
			!strings.Contains(strings.ToLower(t.Title), searchLower) {
			continue
		}
		result = append(result, *t)
	}

	switch sortMode {
	case "alpha":
		sort.Slice(result, func(i, j int) bool {
			return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
		})
	case "due":
		sort.Slice(result, func(i, j int) bool {
			return result[i].DueAt.Before(result[j].DueAt)
		})
	default: // "created"
		sort.Slice(result, func(i, j int) bool {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		})
	}
	return result
}
