import AppKit
import CoreServices
import Darwin

let helperID = "io.github.am-will.multi-codex-app"
struct Profile: Codable { let id: String; let name: String; let codexHome: String; let userDataDir: String }
struct Configuration: Codable { let version: Int; let appPath: String; let cliPath: String; let previousHandler: String?; let profiles: [Profile] }
func rootPath() -> String {
    ProcessInfo.processInfo.environment["MULTI_CODEX_ROOT"] ?? Bundle.main.object(forInfoDictionaryKey: "MultiCodexRoot") as? String ?? NSHomeDirectory() + "/Library/Application Support/Multi Codex"
}
func configuration() throws -> Configuration {
    let c = try JSONDecoder().decode(Configuration.self, from: Data(contentsOf: URL(fileURLWithPath: rootPath() + "/config.json")))
    guard c.version == 1 else { throw NSError(domain: "MultiCodex", code: 1) }; return c
}
func handler() -> String { LSCopyDefaultHandlerForURLScheme("codex" as CFString)?.takeRetainedValue() as String? ?? "" }
func setHandler(_ id: String) -> OSStatus { LSSetDefaultHandlerForURLScheme("codex" as CFString, id as CFString) }
func commandLine(_ pid: pid_t) -> String? {
    let p = Process(); p.executableURL = URL(fileURLWithPath: "/bin/ps"); p.arguments = ["-ww", "-p", String(pid), "-o", "command="]
    let pipe = Pipe(); p.standardOutput = pipe; p.standardError = FileHandle.nullDevice
    do { try p.run() } catch { return nil }
    let data = pipe.fileHandleForReading.readDataToEndOfFile(); p.waitUntilExit()
    guard p.terminationStatus == 0 else { return nil }; return String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
}
func runningInstances(_ c: Configuration) -> [String: NSRunningApplication] {
    let apps = NSWorkspace.shared.runningApplications.filter { $0.bundleURL?.standardizedFileURL.path == URL(fileURLWithPath: c.appPath).standardizedFileURL.path }
    let commands = apps.compactMap { app -> (NSRunningApplication, String)? in
        guard let cmd = commandLine(app.processIdentifier) else { return nil }; return (app, cmd)
    }
    var result: [String: NSRunningApplication] = [:]
    for profile in c.profiles {
        let matches = commands.filter { app, cmd in
            if profile.userDataDir.isEmpty { return !cmd.contains("--user-data-dir") && !cmd.contains("--user-data-path") }
            guard let range = cmd.range(of: "--user-data-dir=" + profile.userDataDir) else { return false }
            let rest = cmd[range.upperBound...]; return rest.isEmpty || rest.hasPrefix(" --")
        }
        if matches.count == 1 { result[profile.id] = matches[0].0 }
    }
    return result
}
func instance(_ profile: Profile, _ c: Configuration) -> NSRunningApplication? { runningInstances(c)[profile.id] }
func deliver(_ raw: String, to app: NSRunningApplication) throws {
    let event = NSAppleEventDescriptor(eventClass: AEEventClass(0x4755524c), eventID: AEEventID(0x4755524c), targetDescriptor: NSAppleEventDescriptor(processIdentifier: app.processIdentifier), returnID: AEReturnID(-1), transactionID: AETransactionID(0))
    event.setParam(NSAppleEventDescriptor(string: raw), forKeyword: AEKeyword(0x2d2d2d2d))
    _ = try event.sendEvent(options: [.waitForReply, .neverInteract], timeout: 5)
}
func validURI(_ raw: String) -> Bool {
    guard raw.utf8.count <= 65536, !raw.contains("\n"), !raw.contains("\r"), !raw.contains("\0"), let u = URLComponents(string: raw) else { return false }
    return u.scheme == "codex" && !(u.host ?? "").isEmpty && u.user == nil && u.password == nil && u.port == nil
}
func showError(_ text: String) {
    let a = NSAlert(); a.messageText = "Connection could not be delivered"; a.informativeText = text; a.addButton(withTitle: "OK"); NSApp.activate(ignoringOtherApps: true); a.runModal()
}

// Cards use ordinary AppKit controls, retaining keyboard and VoiceOver support.
final class ProfileButton: NSButton {
    let profile: Profile
    let available: Bool
    var selected = false { didSet { needsDisplay = true; setAccessibilityValue(selected ? "Selected" : "Not selected") } }
    init(_ profile: Profile, available: Bool) {
        self.profile = profile; self.available = available
        super.init(frame: .zero)
        title = ""; isBordered = false; setButtonType(.momentaryChange); isEnabled = available
        setAccessibilityLabel(profile.name + (available ? ", running" : ", closed"))
        translatesAutoresizingMaskIntoConstraints = false; heightAnchor.constraint(equalToConstant: 64).isActive = true
    }
    required init?(coder: NSCoder) { fatalError("init(coder:)") }
    override func draw(_ dirtyRect: NSRect) {
        let r = bounds.insetBy(dx: 1, dy: 1); let path = NSBezierPath(roundedRect: r, xRadius: 12, yRadius: 12)
        (selected ? NSColor.controlAccentColor.withAlphaComponent(0.10) : NSColor.controlBackgroundColor).setFill(); path.fill()
        (selected ? NSColor.controlAccentColor : NSColor.separatorColor.withAlphaComponent(0.6)).setStroke(); path.lineWidth = selected ? 1.6 : 1; path.stroke()
        let iconRect = NSRect(x: 16, y: 16, width: 32, height: 32)
        let palette: [NSColor] = [.systemBlue, .systemPurple, .systemTeal, .systemOrange, .systemPink]
        let color = palette[((Int(profile.id) ?? 1) - 1) % palette.count]
        color.withAlphaComponent(available ? 0.15 : 0.07).setFill(); NSBezierPath(roundedRect: iconRect, xRadius: 9, yRadius: 9).fill()
        let attrs: [NSAttributedString.Key: Any] = [.font: NSFont.systemFont(ofSize: 15, weight: .semibold), .foregroundColor: available ? color : NSColor.tertiaryLabelColor]
        let number = NSAttributedString(string: profile.id, attributes: attrs); let size = number.size(); number.draw(at: NSPoint(x: iconRect.midX - size.width / 2, y: iconRect.midY - size.height / 2))
        let paragraph = NSMutableParagraphStyle(); paragraph.lineBreakMode = .byTruncatingTail
        NSAttributedString(string: profile.name, attributes: [.font: NSFont.systemFont(ofSize: 14, weight: .semibold), .foregroundColor: available ? NSColor.labelColor : NSColor.secondaryLabelColor, .paragraphStyle: paragraph]).draw(in: NSRect(x: 60, y: 16, width: max(0, bounds.width - 110), height: 20))
        NSAttributedString(string: available ? "Running on this Mac" : "Open this profile to connect", attributes: [.font: NSFont.systemFont(ofSize: 11), .foregroundColor: NSColor.secondaryLabelColor]).draw(at: NSPoint(x: 60, y: 36))
        let circle = NSRect(x: bounds.width - 34, y: 24, width: 16, height: 16)
        if selected {
            NSColor.controlAccentColor.setFill(); NSBezierPath(ovalIn: circle).fill()
            NSColor.white.setStroke(); let check = NSBezierPath(); check.move(to: NSPoint(x: circle.minX + 4, y: circle.midY)); check.line(to: NSPoint(x: circle.minX + 7, y: circle.minY + 11)); check.line(to: NSPoint(x: circle.maxX - 3, y: circle.minY + 5)); check.lineWidth = 1.7; check.stroke()
        } else { NSColor.tertiaryLabelColor.setStroke(); NSBezierPath(ovalIn: circle).stroke() }
        if NSApp.keyWindow?.firstResponder === self { NSColor.keyboardFocusIndicatorColor.setStroke(); let focus = NSBezierPath(roundedRect: bounds.insetBy(dx: 3, dy: 3), xRadius: 10, yRadius: 10); focus.lineWidth = 2; focus.stroke() }
    }
}
final class Chooser: NSObject, NSWindowDelegate {
    var window: NSWindow!
    var buttons: [ProfileButton] = []
    var selected: Profile?
    var completion: ((Profile?) -> Void)?
    var continueButton: NSButton!
    func show(_ c: Configuration, preview: Bool, completion: @escaping (Profile?) -> Void) {
        self.completion = completion
        let listHeight = min(CGFloat(c.profiles.count) * 74, 370)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 460, height: 218 + listHeight), styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.title = "Multi Codex"; window.titleVisibility = .hidden; window.titlebarAppearsTransparent = true; window.isReleasedWhenClosed = false; window.delegate = self
        let view = NSView(); window.contentView = view
        func label(_ text: String, font: NSFont, color: NSColor) -> NSTextField {
            let f = NSTextField(wrappingLabelWithString: text); f.font = font; f.textColor = color; f.translatesAutoresizingMaskIntoConstraints = false; view.addSubview(f); return f
        }
        let title = label("Choose a Codex profile", font: .systemFont(ofSize: 23, weight: .bold), color: .labelColor)
        let subtitle = label("Send this connection to the window where you clicked Connect.", font: .systemFont(ofSize: 13), color: .secondaryLabelColor)
        let scroll = NSScrollView(); scroll.translatesAutoresizingMaskIntoConstraints = false; scroll.drawsBackground = false; scroll.hasVerticalScroller = c.profiles.count > 5; view.addSubview(scroll)
        let stack = NSStackView(); stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 10; stack.translatesAutoresizingMaskIntoConstraints = false; scroll.documentView = stack
        let running = runningInstances(c)
        for p in c.profiles {
            let b = ProfileButton(p, available: preview || running[p.id] != nil); b.target = self; b.action = #selector(selectProfile(_:)); stack.addArrangedSubview(b); b.widthAnchor.constraint(equalTo: stack.widthAnchor).isActive = true; buttons.append(b)
        }
        let cancel = NSButton(title: "Cancel", target: self, action: #selector(cancelChoice)); cancel.bezelStyle = .rounded; cancel.keyEquivalent = "\u{1b}"; cancel.translatesAutoresizingMaskIntoConstraints = false; view.addSubview(cancel)
        continueButton = NSButton(title: preview ? "Done" : "Continue", target: self, action: #selector(confirm)); continueButton.bezelStyle = .rounded; continueButton.keyEquivalent = "\r"; continueButton.isEnabled = false; continueButton.translatesAutoresizingMaskIntoConstraints = false; view.addSubview(continueButton)
        let hint = label(preview ? "Preview — no connection is sent." : "Your other profiles stay open.", font: .systemFont(ofSize: 11), color: .tertiaryLabelColor)
        NSLayoutConstraint.activate([
            title.topAnchor.constraint(equalTo: view.topAnchor, constant: 28), title.leadingAnchor.constraint(equalTo: view.leadingAnchor, constant: 28), title.trailingAnchor.constraint(equalTo: view.trailingAnchor, constant: -28),
            subtitle.topAnchor.constraint(equalTo: title.bottomAnchor, constant: 8), subtitle.leadingAnchor.constraint(equalTo: title.leadingAnchor), subtitle.trailingAnchor.constraint(equalTo: title.trailingAnchor),
            scroll.topAnchor.constraint(equalTo: subtitle.bottomAnchor, constant: 22), scroll.leadingAnchor.constraint(equalTo: title.leadingAnchor), scroll.trailingAnchor.constraint(equalTo: title.trailingAnchor), scroll.heightAnchor.constraint(equalToConstant: listHeight),
            stack.topAnchor.constraint(equalTo: scroll.contentView.topAnchor), stack.leadingAnchor.constraint(equalTo: scroll.contentView.leadingAnchor), stack.trailingAnchor.constraint(equalTo: scroll.contentView.trailingAnchor), stack.bottomAnchor.constraint(equalTo: scroll.contentView.bottomAnchor).withPriority(.defaultLow),
            continueButton.trailingAnchor.constraint(equalTo: title.trailingAnchor), continueButton.topAnchor.constraint(equalTo: scroll.bottomAnchor, constant: 24), continueButton.widthAnchor.constraint(greaterThanOrEqualToConstant: 96), cancel.trailingAnchor.constraint(equalTo: continueButton.leadingAnchor, constant: -10), cancel.centerYAnchor.constraint(equalTo: continueButton.centerYAnchor),
            hint.leadingAnchor.constraint(equalTo: title.leadingAnchor), hint.trailingAnchor.constraint(lessThanOrEqualTo: cancel.leadingAnchor, constant: -12), hint.centerYAnchor.constraint(equalTo: continueButton.centerYAnchor)
        ])
        window.center(); NSApp.activate(ignoringOtherApps: true); window.makeKeyAndOrderFront(nil)
    }
    @objc func selectProfile(_ sender: ProfileButton) { selected = sender.profile; for b in buttons { b.selected = b === sender }; continueButton.isEnabled = true }
    @objc func confirm() { finish(selected) }
    @objc func cancelChoice() { finish(nil) }
    func windowShouldClose(_ sender: NSWindow) -> Bool { finish(nil); return false }
    func finish(_ p: Profile?) { let callback = completion; completion = nil; window.orderOut(nil); callback?(p) }
}
extension NSLayoutConstraint { func withPriority(_ p: NSLayoutConstraint.Priority) -> NSLayoutConstraint { priority = p; return self } }

final class Helper: NSObject, NSApplicationDelegate {
    var status: NSStatusItem!
    var chooser: Chooser?
    var pending: [String] = []
    var ready = false
    var timer: Timer?
    func applicationWillFinishLaunching(_ note: Notification) {
        NSAppleEventManager.shared().setEventHandler(self, andSelector: #selector(receive(_:reply:)), forEventClass: AEEventClass(0x4755524c), andEventID: AEEventID(0x4755524c))
    }
    func applicationDidFinishLaunching(_ note: Notification) {
        if let id = Bundle.main.object(forInfoDictionaryKey: "MultiCodexProfileID") as? String {
            do { let c = try configuration(); let p = Process(); p.executableURL = URL(fileURLWithPath: c.cliPath); p.arguments = ["--root", rootPath(), "launch", id]; p.standardOutput = FileHandle.nullDevice; p.standardError = FileHandle.nullDevice; try p.run(); p.waitUntilExit() } catch { showError("Re-run multi-codex-app setup to repair this launcher.") }; NSApp.terminate(nil); return
        }
        if CommandLine.arguments.contains("--preview") { ready = true; preview(); return }
        status = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        status.button?.image = NSImage(systemSymbolName: "square.stack.3d.up", accessibilityDescription: "Multi Codex profiles")
        rebuildMenu(); claim(); ready = true; processNext()
        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in self?.claim(); self?.rebuildMenu() }
    }
    func claim() { if handler() != helperID { _ = setHandler(helperID) } }
    func rebuildMenu() {
        let menu = NSMenu(); let heading = menu.addItem(withTitle: "Multi Codex", action: nil, keyEquivalent: ""); heading.isEnabled = false; menu.addItem(.separator())
        if let c = try? configuration() { for p in c.profiles { let item = menu.addItem(withTitle: p.name, action: #selector(launchProfile(_:)), keyEquivalent: ""); item.representedObject = p.id; item.target = self } }
        menu.addItem(.separator()); menu.addItem(withTitle: "Preview Connection Chooser…", action: #selector(preview), keyEquivalent: "").target = self
        menu.addItem(withTitle: "About Multi Codex", action: #selector(about), keyEquivalent: "").target = self
        status.menu = menu
    }
    @objc func about() { let a = NSAlert(); a.messageText = "Multi Codex"; a.informativeText = "Independent profiles and connection routing.\n\nInspired by Edi Hasaj's two-account methodology:\nedihasaj.com/posts/two-codex-accounts-two-dock-icons-macos\n\nManage profiles: multi-codex-app add / rename / update\nRemove integration: multi-codex-app uninstall"; a.runModal() }
    @objc func launchProfile(_ sender: NSMenuItem) {
        guard let id = sender.representedObject as? String, let c = try? configuration() else { return }
        let p = Process(); p.executableURL = URL(fileURLWithPath: c.cliPath); p.arguments = ["--root", rootPath(), "launch", id]; p.standardOutput = FileHandle.nullDevice; p.standardError = FileHandle.nullDevice; try? p.run()
    }
    @objc func preview() {
        guard chooser == nil, let c = try? configuration() else { return }; let choice = Chooser(); chooser = choice
        choice.show(c, preview: true) { [weak self] _ in self?.chooser = nil; if CommandLine.arguments.contains("--preview") { NSApp.terminate(nil) } else { self?.processNext() } }
    }
    @objc func receive(_ event: NSAppleEventDescriptor, reply: NSAppleEventDescriptor) {
        guard let raw = event.paramDescriptor(forKeyword: AEKeyword(0x2d2d2d2d))?.stringValue, validURI(raw), pending.count < 10 else { return }
        pending.append(raw); processNext()
    }
    func processNext() {
        guard ready, chooser == nil, !pending.isEmpty, let c = try? configuration() else { return }
        let raw = pending.removeFirst(); let u = URL(string: raw)!
        if u.host == "connector" && u.path == "/oauth_callback" {
            let choice = Chooser(); chooser = choice; choice.show(c, preview: false) { [weak self] profile in
                if let profile { self?.forward(raw, profile, c) }; self?.chooser = nil; self?.processNext()
            }
        } else if let primary = c.profiles.first(where: { $0.id == "1" }) { forward(raw, primary, c); processNext() }
    }
    func forward(_ raw: String, _ p: Profile, _ c: Configuration) {
        guard let app = instance(p, c) else { showError("Open \(p.name), then restart Connect from that profile."); return }
        do { try deliver(raw, to: app); app.activate(options: [.activateAllWindows]) } catch { showError("macOS could not reach \(p.name). Restart Connect from that profile. The callback was not sent to another account.") }
    }
}

func renderIcon(_ id: String, _ destination: String) throws {
    // Build an iconset on demand without shipping Apple's or OpenAI's artwork.
    let temp = URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent(UUID().uuidString + ".iconset"); try FileManager.default.createDirectory(at: temp, withIntermediateDirectories: true); defer { try? FileManager.default.removeItem(at: temp) }
    let palette: [NSColor] = [.systemBlue, .systemPurple, .systemTeal, .systemOrange, .systemPink]; let color = palette[((Int(id) ?? 1) - 1) % palette.count]
    for size in [16,32,64,128,256,512,1024] {
        let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState(); NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        let s = CGFloat(size); let shape = NSBezierPath(roundedRect: NSRect(x: s*0.06,y:s*0.06,width:s*0.88,height:s*0.88),xRadius:s*0.20,yRadius:s*0.20)
        NSGradient(starting: color, ending: color.blended(withFraction: 0.35, of: .black)!)!.draw(in: shape, angle: -75)
        let number = NSAttributedString(string:id,attributes:[.font:NSFont.systemFont(ofSize:s*0.46,weight:.bold),.foregroundColor:NSColor.white]); let ns=number.size(); number.draw(at:NSPoint(x:(s-ns.width)/2,y:(s-ns.height)/2+s*0.025)); NSGraphicsContext.restoreGraphicsState()
        let data=rep.representation(using:.png,properties:[:])!; if size<=512 {try data.write(to:temp.appendingPathComponent("icon_\(size)x\(size).png"))};if size>=32 {let base=size/2;try data.write(to:temp.appendingPathComponent("icon_\(base)x\(base)@2x.png"))}
    }
    let p=Process();p.executableURL=URL(fileURLWithPath:"/usr/bin/iconutil");p.arguments=["-c","icns",temp.path,"-o",destination];try p.run();p.waitUntilExit();if p.terminationStatus != 0 {throw NSError(domain:"MultiCodex",code:2)}
}
func dockPaths(_ removing: Bool, paths: [String]) throws {
    let domain="com.apple.dock" as CFString; let key="persistent-apps" as CFString
    var items=CFPreferencesCopyAppValue(key,domain) as? [[String:Any]] ?? []
    let pathsSet=Set(paths.map{URL(fileURLWithPath:$0).standardizedFileURL.path})
    func pathOf(_ item:[String:Any])->String?{guard let tile=item["tile-data"] as? [String:Any],let data=tile["file-data"] as? [String:Any],let s=data["_CFURLString"] as? String else{return nil};return URL(string:s)?.standardizedFileURL.path}
    let originalCount=items.count
    if removing {items.removeAll{if let p=pathOf($0){return pathsSet.contains(p)};return false}}else{
        var existing=Set(items.compactMap(pathOf))
        for path in paths {let p=URL(fileURLWithPath:path).standardizedFileURL.path;guard !existing.contains(p) else{continue};existing.insert(p);items.append(["tile-type":"file-tile","tile-data":["file-data":["_CFURLString":URL(fileURLWithPath:p).absoluteString,"_CFURLStringType":15],"file-label":URL(fileURLWithPath:p).deletingPathExtension().lastPathComponent,"file-type":41]])}
    }
    guard items.count != originalCount else{return}
    CFPreferencesSetAppValue(key,items as CFPropertyList,domain);guard CFPreferencesAppSynchronize(domain) else{throw NSError(domain:"MultiCodex",code:3)}
    let check=CFPreferencesCopyAppValue(key,domain) as? [[String:Any]] ?? [];guard check.count==items.count else{throw NSError(domain:"MultiCodex",code:4)}
    let p=Process();p.executableURL=URL(fileURLWithPath:"/usr/bin/killall");p.arguments=["Dock"];try p.run();p.waitUntilExit()
}
let args=CommandLine.arguments
if args.contains("--handler"){print(handler());exit(0)}
if args.count >= 4 && args[1]=="--icon" {do{try renderIcon(args[2],args[3]);exit(0)}catch{exit(1)}}
if args.count >= 2 && ["--pin","--unpin"].contains(args[1]) {do{try dockPaths(args[1]=="--unpin",paths:Array(args.dropFirst(2)));exit(0)}catch{exit(1)}}
if args.contains("--restore") {do{let c=try configuration();exit(setHandler(c.previousHandler ?? "com.openai.codex")==0 ? 0:1)}catch{exit(1)}}
if args.contains("--status") {
    do { let c=try configuration();let h=handler();print("Callback handler: \(h)");for p in c.profiles{print("\(p.id) \(p.name): \(instance(p,c).map{"running (PID \($0.processIdentifier))"} ?? "closed")")};exit(h==helperID ? 0:1) }catch{print("Configuration unavailable");exit(1)}
}
if args.count==3 && ["--focus","--deliver"].contains(args[1]) {
    do {let c=try configuration();guard let p=c.profiles.first(where:{$0.id==args[2]}),let app=instance(p,c) else{exit(1)}
        if args[1]=="--deliver"{let raw=String(data:FileHandle.standardInput.readDataToEndOfFile(),encoding:.utf8) ?? "";guard validURI(raw) else{exit(1)};try deliver(raw,to:app)}else{app.activate(options:[.activateAllWindows])};exit(0)
    }catch{exit(1)}
}
// Only the main helper holds the per-installation lock. Utility/launcher modes above exit first.
let lockPath = rootPath() + "/helper.lock"
let lockFD = open(lockPath, O_CREAT | O_RDWR, mode_t(0600))
if !args.contains("--preview") && Bundle.main.object(forInfoDictionaryKey: "MultiCodexProfileID") == nil {
    guard lockFD >= 0 else { exit(1) }
    var acquired = false
    for _ in 0..<30 {
        if flock(lockFD, LOCK_EX | LOCK_NB) == 0 { acquired = true; break }
        usleep(100_000)
    }
    guard acquired else { exit(0) }
}
let app=NSApplication.shared;app.setActivationPolicy(.accessory);let delegate=Helper();app.delegate=delegate;app.run()
