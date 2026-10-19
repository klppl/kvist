// The plugin's words in Swedish.

import type { Catalog } from ".";

export const sv: Catalog = {
  "command.publish": "Publicera nu",
  "command.toggle": "Växla publicering för aktuell anteckning",
  "command.report": "Öppna publiceringsrapporten",
  "command.settingsNote": "Öppna webbplatsens inställningsanteckning",
  "command.openPublished": "Öppna publicerad sida",
  "menu.publish": "Publicera (kvist)",
  "menu.unpublish": "Avpublicera (kvist)",
  "command.layout": "Välj sidlayout",
  "menu.layout": "Sidlayout (kvist)",

  "layout.placeholder": "Välj hur sidan ska se ut på webbplatsen",
  "layout.current": "nuvarande",
  "layout.article": "Artikel",
  "layout.article.desc": "En anteckning med datum, taggar och länkar. Standard.",
  "layout.page": "Sida",
  "layout.page.desc": "En egen sida, för kontakt- eller nu-sidor: titel och beskrivning, utan datum och taggar.",
  "layout.cover": "Omslag",
  "layout.cover.desc": "För inlägg och projekt: anteckningens bild brett ovanför titeln, sedan som en artikel.",
  "layout.about": "Om mig",
  "layout.about.desc": "Ett porträtt, titeln, beskrivningen och dina profillänkar.",
  "layout.intro": "Intro",
  "layout.intro.desc": "För startsidans anteckning: bilden som banner, en stor rubrik och beskrivningen.",

  "status.label": "kvist: {status} (klicka för rapporten)",
  "status.pending": "ändringar väntar",
  "status.publishing": "publicerar…",
  "status.checking": "kontrollerar anteckningar…",
  "status.uploading": "laddar upp {done}/{total}…",
  "status.committing": "publicerar…",
  "status.building": "bygger webbplatsen…",
  "status.buildFailed": "bygget misslyckades",
  "status.published": "publicerad {time}",
  "status.publishedWarnings": { one: "publicerad {time} · {count} varning", other: "publicerad {time} · {count} varningar" },
  "status.stale": "nyare innehåll på servern",
  "status.failed": "publiceringen misslyckades",

  "notice.notConfigured": "kvist: ange serveradress, webbplats och token i inställningarna först.",
  "notice.buildFailed": "kvist: bygget av webbplatsen misslyckades: {error}",
  "notice.upToDate": { one: "kvist: redan uppdaterad ({count} anteckning)", other: "kvist: redan uppdaterad ({count} anteckningar)" },
  "notice.published": {
    one: "kvist: publicerade {count} anteckning ({uploaded} filer uppladdade)",
    other: "kvist: publicerade {count} anteckningar ({uploaded} filer uppladdade)",
  },
  "notice.stale": "kvist: servern har nyare innehåll än den här enheten. Synka ditt valv och använd sedan ”Publicera nu” för att granska.",
  "notice.error": "kvist: {error}",
  "notice.unreachable": "kvist: kunde inte nå servern: {error}",
  "notice.isSettingsNote": "kvist: det här är webbplatsens inställningsanteckning. Den publiceras aldrig som en sida.",
  "notice.staysPublic": "kvist: ”{name}” förblir offentlig: {reason}.",
  "notice.staysPrivate": "kvist: ”{name}” förblir privat: {reason}.",
  "notice.willPublish": "kvist: ”{name}” publiceras vid nästa publicering.",
  "notice.willUnpublish": "kvist: ”{name}” avpubliceras vid nästa publicering.",
  "notice.settingsNoteCreated": "kvist: skapade webbplatsens inställningsanteckning. Fyll i den och publicera sedan.",
  "notice.publishFirst": "kvist: publicera en gång först, så att tillägget vet webbplatsens adress.",
  "notice.notPublished": "kvist: den här anteckningen är inte publicerad.",
  "notice.connected": "kvist: ansluten till ”{site}” ({url}).",
  "notice.waitForPublish": "kvist: vänta tills publiceringen är klar.",
  "notice.layoutSet": "kvist: ”{name}” använder nu layouten {layout}.",
  "notice.layoutNeedsImage": "kvist: lägg till en image- eller cover-egenskap i ”{name}” för bilden.",
  "notice.reset": "kvist: inställningarna har återställts.",

  "reason.excluded_folder": "den ligger i en undantagen mapp",
  "reason.private_tag": "den har den privata taggen",

  "stale.title": "Servern har nyare innehåll",
  "stale.explain":
    "Det betyder oftast att en annan enhet har publicerat ändringar som den här enheten inte har synkat än. Om du publicerar nu återställs dessa äldre versioner:",
  "stale.older": "{path} — äldre version här",
  "stale.missing": "{path} — saknas här, ändrad på servern",
  "stale.more": "…och {count} till",
  "stale.publishAnyway": "Publicera ändå",
  "button.cancel": "Avbryt",

  "report.title": "kvist publiceringsrapport",
  "report.notPublished": "Inte publicerad än.",
  "report.publishOnce": "Publicera en gång för att se vad som är offentligt och vad som förblir privat.",
  "report.public": {
    one: "{count} anteckning och {files} andra filer är offentliga.",
    other: "{count} anteckningar och {files} andra filer är offentliga.",
  },
  "report.links": "Länkar till anteckningar som inte är publicerade",
  "report.linksDesc": "De här länkarna visas som vanlig text på webbplatsen.",
  "report.embeds": "Inbäddningar av anteckningar som inte är publicerade",
  "report.embedsDesc": "De här inbäddningarna utelämnas från sidan.",
  "report.attachments": "Bilagor i undantagna mappar",
  "report.attachmentsDesc": "Publicerade anteckningar hänvisar till dessa, men de förblir privata.",
  "report.types": "Filtyper som webbplatsen inte publicerar",
  "report.typesDesc": "Publicerade anteckningar hänvisar till dessa, men filtypen finns inte i attachment_extensions i serverns konfiguration, så de förblir privata.",
  "report.server": "Från servern",
  "report.serverDesc": "Varningar från den senaste publiceringen och det senaste bygget.",
  "report.section": "{title} ({count})",

  "settings.intro":
    "Servern bestämmer reglerna för publicering. Skapa en token på servern med: kvist token create --site <webbplats> --name <enhet>.",
  "settings.serverURL": "Serveradress",
  "settings.serverURLDesc": "Till exempel https://kvist.example.com",
  "settings.site": "Webbplats",
  "settings.siteDesc": "Webbplatsens id i serverns konfiguration",
  "settings.siteSettings": "Webbplatsinställningar",
  "settings.siteSettingsDesc": "Titel, startsida, meny och färger finns i en anteckning som heter {name}, så att du kan ändra dem från vilken enhet som helst.",
  "settings.attachments": "Mapp för nya bilagor",
  "settings.attachmentsDesc":
    "Var Obsidian sparar bilder och filer du klistrar in eller släpper i en anteckning. Använd en mapp inuti en publik mapp, till exempel den som föreslås, så hålls de samlade och utanför valvets rot. Tomt betyder valvets rot. Det är samma sak som Obsidians \"Standardplats för nya bilagor\".",
  "settings.openSettingsNote": "Öppna inställningsanteckningen",
  "settings.device": "Den här enheten",
  "settings.deviceDesc": "De här inställningarna sparas bara på den här enheten och synkas inte till dina andra enheter.",
  "settings.token": "Token",
  "settings.tokenDesc": "Använd en egen token för varje enhet, så att du kan återkalla en i taget.",
  "settings.deviceName": "Enhetsnamn",
  "settings.deviceNameDesc": "Visas i varningar och på servern.",
  "settings.autoPublish": "Publicera automatiskt",
  "settings.autoPublishDesc":
    "Publicera när ändringarna har lagt sig. Slå bara på det här för en huvudenhet: en enhet som inte har synkat klart kan annars publicera äldre anteckningar.",
  "settings.debounce": "Vänta innan publicering (sekunder)",
  "settings.test": "Testa anslutningen",
  "settings.testButton": "Testa",
  "settings.reset": "Återställ",
  "settings.resetName": "Återställ inställningar",
  "settings.resetDesc":
    "Glöm servern, webbplatsen, token och den här enhetens inställningar, till exempel för att ansluta till en ny webbplats. Dina anteckningar påverkas inte.",
  "settings.resetConfirmTitle": "Återställa kvists inställningar?",
  "settings.resetConfirm":
    "Det här rensar serveradress, webbplats och token, och den här enhetens publiceringsstatus. På andra enheter rensas serveradress och webbplats när valvets inställningar har synkats dit.",
  "settings.resetButton": "Återställ",

  "error.invalidURL": "Ogiltig serveradress: {url}",
  "error.https": "Serveradressen måste börja med https:// (vanlig http går bara för localhost).",
  "error.buildSlow": "Bygget tar lång tid; titta igen senare.",
  "error.protocolPlugin": "Servern talar protokoll {min}–{max}; uppdatera kvist-tillägget.",
  "error.protocolServer": "Servern talar protokoll {min}–{max}; uppdatera kvist-servern.",
  "error.tooManyFiles": "{count} filer att publicera; servern tillåter {max}.",
  "error.tooLarge": "{path} är för stor för att publiceras (gräns {limit} MB).",
  "error.unknownBlob": "Servern bad om en okänd fil ({hash}).",
  "error.staleCancelled": "Publiceringen avbröts: servern har nyare innehåll.",
  "error.settingsNotes": "hittade {count} inställningsanteckningar ({paths}); behåll en",
  "error.caseClash": "”{a}” och ”{b}” skiljer sig bara i stora och små bokstäver; byt namn på en av dem",
  "error.disappeared": "{path} försvann",
  "error.unauthorized": "Servern godtog inte token för den här webbplatsen. Kontrollera token och webbplats i inställningarna.",
  "error.protocol_unsupported": "Tillägget och servern talar olika protokollversioner; uppdatera den äldre.",

  "settingsNote.body": `Inställningar för din kvist-webbplats. Den här anteckningen publiceras aldrig som en sida. Fyll i egenskaperna ovan och publicera sedan. Tomma egenskaper behåller sitt standardvärde.

- **title** och **description**: webbplatsens namn och undertitel.
- **language**: en kod som \`sv\` eller \`en\`, för webbplatsens egna ord och datum.
- **home**: en anteckning som ska vara startsida, skriven som en länk: \`[[Välkommen]]\`. Tom: en genererad startsida.
- **groups**: taggar som grupperar menyn, som \`artiklar\` och \`projekt\`. Tom: dina mappar.
- **primary_links**: länkar högst upp i menyn. **secondary_links**: länkar under Länkar, nedanför dina mappar. Varje punkt är en publicerad anteckning (\`[[Om]]\`), en webbplats (\`https://mastodon.social/@du\`) eller \`Namn: https://…\`. Gör båda till listegenskaper, så att varje länk blir en egen punkt.
- **accent**: länkarnas färg, som \`#3f7d4e\`.
- **footer**: text längst ner i menyn. Markdown fungerar.
- **image**: bilden som visas när en sida delas, som en länk: \`[[förhandsvisning.png]]\`.
- **favicon**: ikonen i webbläsarens flikar, som en länk: \`[[ikon.png]]\`.

Fler inställningar, som en profil med din bild och dina länkar eller besöksstatistik, finns i dokumentationen: https://klppl.github.io/kvist/customizing.html
`,
};
