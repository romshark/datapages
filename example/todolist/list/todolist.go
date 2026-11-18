// Package list provides a domain model of a simple todo list.
package list

import (
	"slices"
	"strconv"
	"strings"
	"sync"
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
	todos  []*Item // In creation order.
	nextID int64
}

func (l *List) AddItem(title, description string, dueAt time.Time) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.nextID++
	l.todos = append(l.todos, &Item{
		ID:          strconv.FormatInt(l.nextID, 16),
		Title:       title,
		Description: description,
		CreatedAt:   time.Now(),
		DueAt:       dueAt,
	})
}

// index returns the position of the item with id in l.todos, or -1.
// The caller must hold l.lock.
func (l *List) index(id string) int {
	return slices.IndexFunc(l.todos, func(t *Item) bool { return t.ID == id })
}

func (l *List) ToggleItem(id string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	i := l.index(id)
	if i < 0 {
		return false
	}
	l.todos[i].Done = !l.todos[i].Done
	return true
}

func (l *List) UpdateItem(
	id, title, description string, done bool, dueAt time.Time,
) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	i := l.index(id)
	if i < 0 {
		return false
	}
	t := l.todos[i]
	t.Title = title
	t.Description = description
	t.Done = done
	t.DueAt = dueAt
	return true
}

func (l *List) DeleteItem(id string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	i := l.index(id)
	if i < 0 {
		return false
	}
	l.todos = slices.Delete(l.todos, i, i+1)
	return true
}

func (l *List) GetItem(id string) (Item, bool) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	i := l.index(id)
	if i < 0 {
		return Item{}, false
	}
	return *l.todos[i], true
}

// ViewParameters selects and orders the items of [List.GetItems].
// The zero value selects all items in creation order.
type ViewParameters struct {
	Search string // Case-insensitive substring of the title.
	Filter string // "all", "done" or "pending". Empty means "all".
	Sort   string // "alpha", "created" or "due". Empty means "created".
}

// GetItems returns copies of the items vp selects, in the order of vp.Sort.
// Items that sort equal stay in creation order.
func (l *List) GetItems(vp ViewParameters) []Item {
	l.lock.RLock()
	defer l.lock.RUnlock()

	var result []Item
	search := strings.ToLower(vp.Search)
	for _, t := range l.todos {
		switch vp.Filter {
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
			!strings.Contains(strings.ToLower(t.Title), search) {
			continue
		}
		result = append(result, *t)
	}

	// "created" needs no sort because l.todos is in creation order.
	switch vp.Sort {
	case "alpha":
		slices.SortStableFunc(result, func(a, b Item) int {
			return strings.Compare(
				strings.ToLower(a.Title), strings.ToLower(b.Title),
			)
		})
	case "due":
		slices.SortStableFunc(result, func(a, b Item) int {
			return a.DueAt.Compare(b.DueAt)
		})
	}
	return result
}
