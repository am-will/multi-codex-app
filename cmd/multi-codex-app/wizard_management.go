package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

func (w *wizardPrompt) menu(question string, labels []string) (int, error) {
	s := styleFor(w.out)
	for i, label := range labels {
		fmt.Fprintf(w.out, "     %s  %s\n", s.accent(strconv.Itoa(i+1)), label)
	}
	fmt.Fprintln(w.out, s.dim("     0  Back / exit"))
	for {
		answer, e := w.ask(question, "0")
		if e != nil {
			return 0, e
		}
		n, e := strconv.Atoi(answer)
		if e == nil && n >= 0 && n <= len(labels) {
			return n, nil
		}
		fmt.Fprintln(w.out, "     Choose one of the displayed numbers.")
	}
}
func (w *wizardPrompt) managementAction(c Config) (string, error) {
	for {
		w.section("02", "WHAT WOULD YOU LIKE TO DO?")
		n, e := w.menu("Choose an action", []string{"Full setup — choose total instances, then name and color them", "Edit one existing profile — pick its name and/or color", "Add or remove one profile"})
		if e != nil {
			return "", e
		}
		switch n {
		case 0:
			return "quit", nil
		case 1:
			return "setup", nil
		case 2:
			if len(c.Profiles) > 0 {
				return "edit", nil
			}
			fmt.Fprintln(w.out, "     No existing profiles to edit yet.")
		case 3:
			w.section("02", "ADD OR REMOVE ONE")
			option, e := w.menu("Choose an action", []string{"Add one profile — choose its number, name, and color", "Remove one profile — keep its saved data"})
			if e != nil {
				return "", e
			}
			if option == 1 {
				return "add", nil
			}
			if option == 2 {
				if len(c.Profiles) > 0 {
					return "remove", nil
				}
				fmt.Fprintln(w.out, "     No existing profiles to remove yet.")
			}
		}
	}
}
func (w *wizardPrompt) pickProfile(c Config) (Profile, bool, error) {
	w.section("03", "SELECT A PROFILE")
	s := styleFor(w.out)
	for _, p := range c.Profiles {
		fmt.Fprintf(w.out, "     %s  %s  %s\n", s.accent("#"+p.ID), s.paint("1", launcherName(p.Name)), s.icon(iconColor(p)))
	}
	fmt.Fprintln(w.out, s.dim("     0  Back / exit"))
	for {
		value, e := w.ask("Choose the profile number", "0")
		if e != nil {
			return Profile{}, false, e
		}
		if value == "0" {
			return Profile{}, false, nil
		}
		p, e := profile(c, value)
		if e == nil {
			return p, true, nil
		}
		fmt.Fprintln(w.out, "     Choose an existing profile from the list.")
	}
}
func (w *wizardPrompt) reviewProfile(p Profile, action string) (bool, error) {
	w.section("04", action)
	s := styleFor(w.out)
	fmt.Fprintf(w.out, "     %s  %s  %s\n", s.dim("#"+p.ID), s.paint("1", launcherName(p.Name)), s.icon(iconColor(p)))
	fmt.Fprintln(w.out, s.dim("     Saved sign-ins and profile data are retained."))
	return w.yesNo("Apply this change?", true)
}
func (w *wizardPrompt) editExisting(root string, c Config) error {
	p, ok, e := w.pickProfile(c)
	if e != nil || !ok {
		return e
	}
	field, e := w.menu("What would you like to change?", []string{"Name only", "Icon color only", "Name and icon color"})
	if e != nil || field == 0 {
		return e
	}
	c.Profiles = append([]Profile(nil), c.Profiles...)
	for i := range c.Profiles {
		if c.Profiles[i].ID == p.ID {
			if field == 2 || field == 3 {
				w.palette()
			}
			if e = w.editFields(&c, i, field == 1 || field == 3, field == 2 || field == 3); e != nil {
				return e
			}
			updated := c.Profiles[i]
			if p.Name == updated.Name && iconColor(p) == iconColor(updated) {
				fmt.Fprintln(w.out, "     Nothing changed.")
				return nil
			}
			apply, e := w.reviewProfile(updated, "UPDATE THIS PROFILE")
			if e != nil || !apply {
				return e
			}
			latest, err := readConfig(root)
			if err != nil {
				return err
			}
			current, err := profile(latest, p.ID)
			if err != nil {
				return err
			}
			if ((field == 1 || field == 3) && current.Name != p.Name) || ((field == 2 || field == 3) && iconColor(current) != iconColor(p)) {
				return errors.New("this profile changed while the wizard was open; select it again")
			}
			for j := range latest.Profiles {
				if latest.Profiles[j].ID == p.ID {
					if field == 1 || field == 3 {
						latest.Profiles[j].Name = updated.Name
					}
					if field == 2 || field == 3 {
						latest.Profiles[j].IconColor = updated.IconColor
					}
				}
			}
			c = latest
			if e = applyAppearance(root, &c); e != nil {
				return e
			}
			fmt.Fprintf(w.out, "     Updated #%s: %s · %s\n", updated.ID, launcherName(updated.Name), iconColor(updated))
			return nil
		}
	}
	return errors.New("selected profile is no longer available")
}
func (w *wizardPrompt) addSingle(root string, c Config, app string, pin bool) error {
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	if len(c.Profiles) >= 100 {
		return errors.New("at most 100 active profiles are supported")
	}
	next, e := nextProfileID(c, root)
	if e != nil {
		return e
	}
	w.section("03", "ADD ONE PROFILE")
	c.Profiles = append([]Profile(nil), c.Profiles...)
	for {
		number, e := w.ask("Profile number", next)
		if e != nil {
			return e
		}
		if e = appendProfile(&c, number, root, home); e == nil {
			break
		}
		fmt.Fprintln(w.out, "     "+e.Error())
	}
	index := len(c.Profiles) - 1
	w.palette()
	if e = w.editFields(&c, index, true, true); e != nil {
		return e
	}
	if app != "" {
		c.AppPath, e = filepath.Abs(app)
		if e != nil {
			return e
		}
	}
	if c.AppPath == "" {
		c.AppPath, e = detectApp()
		if e != nil {
			c.AppPath, e = w.ask("Path to the installed Codex desktop app", "")
			if e == nil {
				c.AppPath, e = filepath.Abs(c.AppPath)
			}
		}
		if e != nil {
			return e
		}
	}
	if e = checkApp(c.AppPath); e != nil {
		return e
	}
	if runtime.GOOS == "darwin" && pin {
		pin, e = w.yesNo("Add this launcher to the Dock?", true)
		if e != nil {
			return e
		}
	}
	apply, e := w.reviewProfile(c.Profiles[index], "ADD THIS PROFILE")
	if e != nil || !apply {
		return e
	}
	newProfile := c.Profiles[index]
	if latest, err := readConfig(root); err == nil {
		if err = appendProfile(&latest, newProfile.ID, root, home); err != nil {
			return err
		}
		latest.Profiles[len(latest.Profiles)-1] = newProfile
		if app != "" {
			latest.AppPath = c.AppPath
		}
		c = latest
	} else if !os.IsNotExist(err) {
		return err
	}
	return persistInstallation(root, &c, pin, newProfile.ID)
}
func (w *wizardPrompt) removeExisting(root string, c Config) error {
	p, ok, e := w.pickProfile(c)
	if e != nil || !ok {
		return e
	}
	w.section("04", "REMOVE THIS PROFILE")
	fmt.Fprintf(w.out, "     #%s  %s\n", p.ID, launcherName(p.Name))
	fmt.Fprintln(w.out, "     Its launcher and helper entry will be removed. Saved data stays in place.")
	if len(c.Profiles) == 1 {
		fmt.Fprintln(w.out, "     With no profiles left, helper integration will also be uninstalled.")
	}
	yes, e := w.yesNo("Remove this profile?", false)
	if e != nil || !yes {
		return e
	}
	return removeProfileAt(root, c, p.ID, "")
}
