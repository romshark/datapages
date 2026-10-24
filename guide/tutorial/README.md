# Tutorial

Build a shared shopping list with sign-in and an HTTP test. Every open tab shows the same list. A change in 1 tab updates all of them.

You need Go, templ v0.3.1070, the `datapages` CLI and Docker, which [Installation](../installation.md) covers. The tutorial assumes you know Go and `net/http`. It introduces templ and Datastar where the code first uses them.

At the end, the app:

- shows the list in every open tab and updates each tab without a reload
- adds, checks off, renames and removes items
- keeps the rename field of 1 tab open while other tabs change the list
- lets signed-in users change the list and shows it read-only to guests
- passes a test that signs in and adds an item over HTTP

## Steps

Each step changes the code of the step before it.

1. [Create the project](create-project.md)
2. [Show the list](show-the-list.md)
3. [Add items](add-items.md)
4. [Update every open tab](update-every-tab.md)
5. [Check off and remove items](check-off-and-remove.md)
6. [Rename an item with per-tab state](rename-items.md)
7. [Sign in](sign-in.md)
8. [Test the app](test-the-app.md)
