"""Display profiles; print only the chosen ID. No callback URLs reach this process."""
import json
import sys
import tkinter as tk
from tkinter import ttk

with open(sys.argv[1], encoding="utf-8") as file:
    profiles = json.load(file)["profiles"]
root = tk.Tk()
root.title("Multi Codex")
root.configure(bg="#f4f5f8")
root.geometry("460x480")
root.minsize(420, 350)
frame = tk.Frame(root, bg="#f4f5f8", padx=28, pady=24)
frame.pack(fill="both", expand=True)
tk.Label(frame, text="Choose a Codex profile", font=("sans-serif", 19, "bold"), bg="#f4f5f8", fg="#171a25").pack(anchor="w")
tk.Label(frame, text="Use the profile where you clicked Connect.", font=("sans-serif", 11), bg="#f4f5f8", fg="#62677a").pack(anchor="w", pady=(8, 20))
listbox = tk.Listbox(frame, font=("sans-serif", 13), bg="white", fg="#171a25", selectbackground="#dce8ff", selectforeground="#152f65", activestyle="none", borderwidth=0, highlightthickness=1, highlightbackground="#dde0e9", exportselection=False)
for profile in profiles:
    listbox.insert("end", "  {}   {}".format(profile["id"], profile["name"]))
listbox.pack(fill="both", expand=True)
footer = tk.Frame(frame, bg="#f4f5f8")
footer.pack(fill="x", pady=(20, 0))

def submit():
    selection = listbox.curselection()
    if selection:
        print(profiles[selection[0]]["id"], flush=True)
        root.destroy()

button = ttk.Button(footer, text="Continue", command=submit, state="disabled")
button.pack(side="right")
ttk.Button(footer, text="Cancel", command=root.destroy).pack(side="right", padx=8)
listbox.bind("<<ListboxSelect>>", lambda event: button.configure(state="normal" if listbox.curselection() else "disabled"))
root.bind("<Escape>", lambda event: root.destroy())
root.bind("<Return>", lambda event: submit())
root.mainloop()
