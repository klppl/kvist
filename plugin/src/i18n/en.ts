// The plugin's words in English: the source of every key. Add new keys
// here first; other languages fall back to these.

export const en = {
  // Commands and menus
  "command.publish": "Publish now",
  "command.toggle": "Toggle publish for current note",
  "command.report": "Open publish report",
  "command.settingsNote": "Open site settings note",
  "command.openPublished": "Open published page",
  "menu.publish": "Publish (kvist)",
  "menu.unpublish": "Unpublish (kvist)",
  "command.layout": "Set page layout",
  "menu.layout": "Page layout (kvist)",

  // Page layouts (the layout property), in the order of LAYOUTS
  "layout.placeholder": "Pick how this page is laid out on the site",
  "layout.current": "current",
  "layout.article": "Article",
  "layout.article.desc": "A note with its dates, tags and links. The default.",
  "layout.page": "Page",
  "layout.page.desc": "A page of its own, for contact or now pages: the title and description, no dates or tags.",
  "layout.cover": "Cover",
  "layout.cover.desc": "For posts and projects: the note's image wide above the title, then like an article.",
  "layout.about": "About me",
  "layout.about.desc": "A portrait, the title, the description and your profile links.",
  "layout.intro": "Intro",
  "layout.intro.desc": "For the home note: the image as a banner, a large title and the description.",

  // Status bar (lowercase: it follows "● kvist" style text)
  "status.label": "kvist: {status} (click for the report)",
  "status.pending": "changes pending",
  "status.publishing": "publishing…",
  "status.checking": "checking notes…",
  "status.uploading": "uploading {done}/{total}…",
  "status.committing": "publishing…",
  "status.building": "building the site…",
  "status.buildFailed": "build failed",
  "status.published": "published {time}",
  "status.publishedWarnings": { one: "published {time} · {count} warning", other: "published {time} · {count} warnings" },
  "status.stale": "newer content on the server",
  "status.failed": "publish failed",

  // Notices
  "notice.notConfigured": "kvist: set the server URL, site and token in the settings first.",
  "notice.buildFailed": "kvist: the site build failed: {error}",
  "notice.upToDate": { one: "kvist: up to date ({count} note)", other: "kvist: up to date ({count} notes)" },
  "notice.published": {
    one: "kvist: published {count} note ({uploaded} files uploaded)",
    other: "kvist: published {count} notes ({uploaded} files uploaded)",
  },
  "notice.stale": "kvist: the server has newer content than this device. Sync your vault, then use “Publish now” to review.",
  "notice.error": "kvist: {error}",
  "notice.unreachable": "kvist: could not reach the server: {error}",
  "notice.isSettingsNote": "kvist: this is the site's settings note. It is never published as a page.",
  "notice.staysPublic": "kvist: “{name}” stays public: {reason}.",
  "notice.staysPrivate": "kvist: “{name}” stays private: {reason}.",
  "notice.willPublish": "kvist: “{name}” will be published on the next publish.",
  "notice.willUnpublish": "kvist: “{name}” will be unpublished on the next publish.",
  "notice.settingsNoteCreated": "kvist: created the site settings note. Fill it in, then publish.",
  "notice.publishFirst": "kvist: publish once first, so the plugin knows the site address.",
  "notice.notPublished": "kvist: this note is not published.",
  "notice.connected": "kvist: connected to “{site}” ({url}).",
  "notice.waitForPublish": "kvist: wait for the publish to finish.",
  "notice.reset": "kvist: settings reset.",
  "notice.layoutSet": "kvist: “{name}” now uses the {layout} layout.",
  "notice.layoutNeedsImage": "kvist: add an image or cover property to “{name}” for the picture.",

  // Why a rule overrides the publish property
  "reason.excluded_folder": "it is in an excluded folder",
  "reason.private_tag": "it has the private tag",

  // Stale content dialog
  "stale.title": "The server has newer content",
  "stale.explain":
    "This usually means another device published changes this device hasn't synced yet. Publishing now would restore these older versions:",
  "stale.older": "{path} — older version here",
  "stale.missing": "{path} — missing here, changed on the server",
  "stale.more": "…and {count} more",
  "stale.publishAnyway": "Publish anyway",
  "button.cancel": "Cancel",

  // Publish report
  "report.title": "kvist publish report",
  "report.notPublished": "Not published yet.",
  "report.publishOnce": "Publish once to see what is public and what stays private.",
  "report.public": {
    one: "{count} note and {files} other files are public.",
    other: "{count} notes and {files} other files are public.",
  },
  "report.links": "Links to notes that are not published",
  "report.linksDesc": "These links show as plain text on the site.",
  "report.embeds": "Embeds of notes that are not published",
  "report.embedsDesc": "These embeds are left out of the page.",
  "report.attachments": "Attachments in excluded folders",
  "report.attachmentsDesc": "Published notes reference these, but they stay private.",
  "report.types": "File types the site doesn't publish",
  "report.typesDesc": "Published notes reference these, but their file type isn't in attachment_extensions in the server's config, so they stay private.",
  "report.server": "From the server",
  "report.serverDesc": "Warnings from the last publish and build.",
  "report.section": "{title} ({count})",

  // Settings
  "settings.intro":
    "The server decides the publish rules. Create a token on the server with: kvist token create --site <site> --name <device>.",
  "settings.serverURL": "Server URL",
  "settings.serverURLDesc": "For example https://kvist.example.com",
  "settings.site": "Site",
  "settings.siteDesc": "The site id from the server config",
  "settings.siteSettings": "Site settings",
  "settings.siteSettingsDesc": "Title, home page, menu and colors live in a note called {name}, so you can change them from any device.",
  "settings.attachments": "Folder for new attachments",
  "settings.attachmentsDesc":
    "Where Obsidian saves images and files you paste or drop into a note. Use a folder inside a public folder, for example the one suggested, to keep them together and out of the vault root. Empty means the vault root. This is the same as Obsidian's \"Default location for new attachments\".",
  "settings.openSettingsNote": "Open settings note",
  "settings.device": "This device",
  "settings.deviceDesc": "These settings are stored on this device only and are not synced to your other devices.",
  "settings.token": "Token",
  "settings.tokenDesc": "Use a separate token per device, so you can revoke one.",
  "settings.deviceName": "Device name",
  "settings.deviceNameDesc": "Shown in warnings and on the server.",
  "settings.autoPublish": "Publish automatically",
  "settings.autoPublishDesc":
    "Publish after changes settle. Turn this on for one main device only: a device that hasn't finished syncing could otherwise publish older notes.",
  "settings.debounce": "Wait before publishing (seconds)",
  "settings.test": "Test connection",
  "settings.testButton": "Test",
  "settings.reset": "Reset",
  "settings.resetName": "Reset settings",
  "settings.resetDesc":
    "Forget the server, site, token and this device's settings, for example to connect to a new site. Your notes are not touched.",
  "settings.resetConfirmTitle": "Reset kvist settings?",
  "settings.resetConfirm":
    "This clears the server URL, site and token, and this device's publish state. On other devices, the server URL and site are cleared once this vault's settings sync to them.",
  "settings.resetButton": "Reset",

  // Errors the plugin finds itself
  "error.invalidURL": "Invalid server URL: {url}",
  "error.https": "The server URL must use https:// (plain http is only allowed for localhost).",
  "error.buildSlow": "The build is taking long; check again later.",
  "error.protocolPlugin": "The server speaks protocol {min}–{max}; update the kvist plugin.",
  "error.protocolServer": "The server speaks protocol {min}–{max}; update the kvist server.",
  "error.tooManyFiles": "{count} files to publish; the server allows {max}.",
  "error.tooLarge": "{path} is too large to publish (limit {limit} MB).",
  "error.unknownBlob": "The server asked for an unknown file ({hash}).",
  "error.staleCancelled": "Publishing was cancelled: the server has newer content.",
  "error.settingsNotes": "found {count} settings notes ({paths}); keep one",
  "error.caseClash": "“{a}” and “{b}” differ only by case; rename one",
  "error.disappeared": "{path} disappeared",
  // Server errors by code; the server's own (English) message is used for the rest.
  "error.unauthorized": "The server did not accept the token for this site. Check the token and the site in the settings.",
  "error.protocol_unsupported": "The plugin and the server speak different protocol versions; update the older one.",

  // The settings note (_site.md), below its properties. The property
  // names in it stay in English: they are what the server reads.
  "settingsNote.body": `Settings for your kvist website. This note is never published as a page. Fill in the properties above, then publish. Empty ones keep their default.

- **title** and **description**: the site's name and tagline.
- **language**: a code such as \`en\` or \`sv\`, for the site's own words and dates.
- **home**: a note to use as the home page, written as a link: \`[[Welcome]]\`. Empty: a generated home page.
- **groups**: tags that group the menu, such as \`articles\` and \`projects\`. Empty: your folders.
- **primary_links**: links at the top of the menu. **secondary_links**: links under Links, below your folders. Each item is a published note (\`[[About]]\`), a website (\`https://mastodon.social/@you\`) or \`Name: https://…\`. Make both List properties, so each link is its own item.
- **accent**: the color of links, such as \`#3f7d4e\`.
- **footer**: text at the bottom of the menu. Markdown works.
- **image**: the picture shown when a page is shared, as a link: \`[[preview.png]]\`.
- **favicon**: the icon in browser tabs, as a link: \`[[icon.png]]\`.

More settings, such as a profile with your picture and links, or visitor statistics, are in the documentation: https://klppl.github.io/kvist/customizing.html
`,
} satisfies Record<string, string | { other: string; one?: string }>;
