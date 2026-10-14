package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/users"
)

const usersUsage = `usage:
  adx users list
  adx users find-account --email EMAIL
  adx users add --email EMAIL --role viewer|marketer|admin [--authentik-uuid UUID]
  adx users set-role (--id ID | --email EMAIL) --role viewer|marketer|admin
  adx users password-link (--id ID | --email EMAIL)
  adx users disable (--id ID | --email EMAIL)
  adx users enable (--id ID | --email EMAIL)

To add someone who already has an authentik account, run find-account,
check it's the right person, then add with that account's --authentik-uuid`

// runUsers manages people from the server's shell, such as adding the first
// admin or recovering from a mistake. It goes through the same core functions
// and audit log as the admin page, acting as ADX itself.
func runUsers(ctx context.Context, u *users.Users, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usersUsage)
	}
	actor := core.System(core.ViaCLI)

	fs := flag.NewFlagSet("adx users "+args[0], flag.ContinueOnError)
	fs.SetOutput(out)
	email := fs.String("email", "", "the person's email in ADX")
	id := fs.Int64("id", 0, "the person's ID in ADX, as shown by list")
	role := fs.String("role", "", "viewer, marketer or admin")
	authentikUUID := fs.String("authentik-uuid", "", "link this existing authentik account, as shown by find-account")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	// target resolves --id or --email to the person's ID.
	target := func() (int64, error) {
		switch {
		case *id != 0 && *email != "":
			return 0, errors.New("give --id or --email, not both")
		case *id != 0:
			return *id, nil
		}
		user, err := u.FindByEmail(ctx, actor, *email)
		return user.ID, err
	}

	var (
		user users.User
		err  error
	)
	switch args[0] {
	case "list":
		list, err := u.List(ctx, actor)
		if err != nil {
			return err
		}
		printUsers(out, list...)
		return nil
	case "find-account":
		found, err := u.FindAccounts(ctx, actor, *email)
		if err != nil {
			return err
		}
		printAccounts(out, found)
		return nil
	case "add":
		user, err = u.Add(ctx, actor, users.AddInput{Email: *email, Role: core.Role(*role), LinkUUID: *authentikUUID})
	case "set-role":
		var who int64
		if who, err = target(); err == nil {
			user, err = u.SetRole(ctx, actor, who, core.Role(*role))
		}
	case "password-link":
		who, err := target()
		if err != nil {
			return err
		}
		if err := u.SendPasswordLink(ctx, actor, who); err != nil {
			return err
		}
		fmt.Fprintln(out, "sent the set-password email")
		return nil
	case "disable":
		var who int64
		if who, err = target(); err == nil {
			user, err = u.Disable(ctx, actor, who)
		}
	case "enable":
		var who int64
		if who, err = target(); err == nil {
			user, err = u.Enable(ctx, actor, who)
		}
	default:
		return errors.New(usersUsage)
	}
	// Some failures still change things, such as an add whose email didn't
	// send, so show the person whenever there is one.
	if user.ID != 0 {
		printUsers(out, user)
	}
	return err
}

func printUsers(out io.Writer, list ...users.User) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tSTATUS\tAUTHENTIK UUID")
	for _, u := range list {
		name := "-"
		if u.Name != nil {
			name = *u.Name
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", u.ID, u.Email, name, u.Role, u.Status, u.AuthentikUUID)
	}
	w.Flush()
}

func printAccounts(out io.Writer, list []users.Account) {
	if len(list) == 0 {
		fmt.Fprintln(out, "no authentik account has this email, so add creates one")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tEMAIL\tUUID\tACTIVE\tMADE BY ADX")
	for _, a := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\n", a.Username, a.Email, a.UUID, a.Active, a.CreatedByADX)
	}
	w.Flush()
}
