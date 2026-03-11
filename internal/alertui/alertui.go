package alertui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"neurader/internal/config"
)

// NeuRader green theme
var (
	colorBg     = tcell.NewRGBColor(0, 30, 0)       // dark green background
	colorFg     = tcell.NewRGBColor(0, 255, 80)      // bright green text
	colorTitle  = tcell.NewRGBColor(0, 200, 60)      // title green
	colorSelect = tcell.NewRGBColor(0, 180, 50)      // selected item
	colorBorder = tcell.NewRGBColor(0, 150, 40)      // border green
	colorDim    = tcell.NewRGBColor(0, 120, 30)      // dimmed text
	colorWhite  = tcell.NewRGBColor(220, 255, 220)   // near-white for input text
	colorRed    = tcell.NewRGBColor(255, 80, 80)     // error/quit
)

// Run launches the interactive alert configuration TUI.
func Run(cfg config.Config) (config.Config, bool, error) {
	app := tview.NewApplication()

	var saved bool
	var finalCfg config.Config = cfg

	// Show main menu first
	mainMenu(app, &finalCfg, &saved)

	if err := app.Run(); err != nil {
		return cfg, false, fmt.Errorf("tui: %w", err)
	}

	return finalCfg, saved, nil
}

// ── Main Menu ─────────────────────────────────────────────────────────────────

func mainMenu(app *tview.Application, cfg *config.Config, saved *bool) {
	list := tview.NewList()
	styleList(list)
	list.SetTitle("  NeuRader — Alert Setup  ")
	list.SetBorder(true)
	list.SetTitleColor(colorFg)
	list.SetBorderColor(colorBorder)
	list.SetBackgroundColor(colorBg)
	list.SetMainTextColor(colorFg)
	list.SetSelectedBackgroundColor(colorSelect)
	list.SetSelectedTextColor(colorBg)
	list.SetSecondaryTextColor(colorDim)

	list.AddItem("Configure Slack",                  configuredMark(cfg.SlackWebhook != ""),              'S', nil)
	list.AddItem("Configure PagerDuty",              configuredMark(cfg.PagerDutyRoutingKey != ""),       'P', nil)
	list.AddItem("Configure Microsoft Teams",        configuredMark(cfg.TeamsWebhook != ""),              'T', nil)
	list.AddItem("Configure Jira",                   configuredMark(cfg.JiraURL != ""),                   'J', nil)
	list.AddItem("Configure Email",                  configuredMark(cfg.EmailSMTPHost != ""),             'E', nil)
	list.AddItem("Configure Telegram",               configuredMark(cfg.TelegramBotToken != ""),          'G', nil)
	list.AddItem("Configure Webhook",                configuredMark(cfg.WebhookURL != ""),                'W', nil)
	list.AddItem("Configure Prometheus Alertmanager",configuredMark(cfg.AlertmanagerURL != ""),           'A', nil)
	list.AddItem("", "", 0, nil) // spacer
	list.AddItem("Quit", "", 'Q', nil)

	list.SetSelectedFunc(func(index int, main, secondary string, shortcut rune) {
		switch index {
		case 0:
			showSlackForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 1:
			showPagerDutyForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 2:
			showTeamsForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 3:
			showJiraForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 4:
			showEmailForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 5:
			showTelegramForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 6:
			showWebhookForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 7:
			showAlertmanagerForm(app, cfg, saved, func() { mainMenu(app, cfg, saved) })
		case 9: // Quit (after spacer)
			app.Stop()
		}
	})

	// Center the menu on screen
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(list, 22, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)

	flex.SetBackgroundColor(colorBg)
	app.SetRoot(flex, true)
}

// ── Form helpers ──────────────────────────────────────────────────────────────

func newForm(title string) *tview.Form {
	form := tview.NewForm()
	form.SetTitle(fmt.Sprintf("  %s  ", title))
	form.SetBorder(true)
	form.SetTitleColor(colorFg)
	form.SetBorderColor(colorBorder)
	form.SetBackgroundColor(colorBg)
	form.SetFieldBackgroundColor(tcell.NewRGBColor(0, 50, 0))
	form.SetFieldTextColor(colorWhite)
	form.SetLabelColor(colorFg)
	form.SetButtonBackgroundColor(colorSelect)
	form.SetButtonTextColor(colorBg)
	return form
}

func wrapForm(app *tview.Application, form *tview.Form, height int) {
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, height, 0, true).
			AddItem(nil, 0, 1, false), 65, 0, true).
		AddItem(nil, 0, 1, false)
	flex.SetBackgroundColor(colorBg)
	app.SetRoot(flex, true)
}

func saveAndBack(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	if err := config.Save(*cfg); err != nil {
		showError(app, fmt.Sprintf("Save failed: %v", err), back)
		return
	}
	*saved = true
	back()
}

func showError(app *tview.Application, msg string, back func()) {
	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(idx int, label string) { back() })
	modal.SetBackgroundColor(colorBg)
	modal.SetTextColor(colorRed)
	modal.SetButtonBackgroundColor(colorSelect)
	modal.SetButtonTextColor(colorBg)
	app.SetRoot(modal, true)
}

func styleList(list *tview.List) {
	list.ShowSecondaryText(true)
}

func configuredMark(configured bool) string {
	if configured {
		return "✓ configured"
	}
	return "not configured"
}

// ── Slack ─────────────────────────────────────────────────────────────────────

func showSlackForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Slack Configuration")
	webhookURL := cfg.SlackWebhook

	form.AddInputField("Webhook URL", webhookURL, 50, nil, func(v string) { webhookURL = v })
	form.AddButton("Save", func() {
		cfg.SlackWebhook = webhookURL
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.SlackWebhook = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 10)
}

// ── PagerDuty ─────────────────────────────────────────────────────────────────

func showPagerDutyForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("PagerDuty Configuration")
	routingKey := cfg.PagerDutyRoutingKey

	form.AddInputField("Routing Key", routingKey, 50, nil, func(v string) { routingKey = v })
	form.AddButton("Save", func() {
		cfg.PagerDutyRoutingKey = routingKey
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.PagerDutyRoutingKey = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 10)
}

// ── Microsoft Teams ───────────────────────────────────────────────────────────

func showTeamsForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Microsoft Teams Configuration")
	webhookURL := cfg.TeamsWebhook

	form.AddInputField("Webhook URL", webhookURL, 50, nil, func(v string) { webhookURL = v })
	form.AddButton("Save", func() {
		cfg.TeamsWebhook = webhookURL
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.TeamsWebhook = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 10)
}

// ── Jira ──────────────────────────────────────────────────────────────────────

func showJiraForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Jira Configuration")
	jiraURL     := cfg.JiraURL
	jiraUser    := cfg.JiraUser
	jiraToken   := cfg.JiraToken
	jiraProject := cfg.JiraProject

	form.AddInputField("Jira URL",       jiraURL,     50, nil, func(v string) { jiraURL = v })
	form.AddInputField("User Email",     jiraUser,    50, nil, func(v string) { jiraUser = v })
	form.AddPasswordField("API Token",   jiraToken,   50, '*', func(v string) { jiraToken = v })
	form.AddInputField("Project Key",    jiraProject, 10, nil, func(v string) { jiraProject = v })
	form.AddButton("Save", func() {
		cfg.JiraURL     = jiraURL
		cfg.JiraUser    = jiraUser
		cfg.JiraToken   = jiraToken
		cfg.JiraProject = jiraProject
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.JiraURL     = ""
		cfg.JiraUser    = ""
		cfg.JiraToken   = ""
		cfg.JiraProject = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 16)
}

// ── Email ─────────────────────────────────────────────────────────────────────

func showEmailForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Email Configuration")
	smtpHost := cfg.EmailSMTPHost
	smtpPort := cfg.EmailSMTPPort
	from      := cfg.EmailFrom
	to        := cfg.EmailTo
	password  := cfg.EmailPassword

	form.AddInputField("SMTP Host",  smtpHost, 40, nil, func(v string) { smtpHost = v })
	form.AddInputField("SMTP Port",  smtpPort, 6,  nil, func(v string) { smtpPort = v })
	form.AddInputField("From",       from,     40, nil, func(v string) { from = v })
	form.AddInputField("To",         to,       40, nil, func(v string) { to = v })
	form.AddPasswordField("Password",password, 40, '*', func(v string) { password = v })
	form.AddButton("Save", func() {
		cfg.EmailSMTPHost = smtpHost
		cfg.EmailSMTPPort = smtpPort
		cfg.EmailFrom     = from
		cfg.EmailTo       = to
		cfg.EmailPassword = password
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.EmailSMTPHost = ""
		cfg.EmailSMTPPort = ""
		cfg.EmailFrom     = ""
		cfg.EmailTo       = ""
		cfg.EmailPassword = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 18)
}

// ── Telegram ──────────────────────────────────────────────────────────────────

func showTelegramForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Telegram Configuration")
	botToken := cfg.TelegramBotToken
	chatID   := cfg.TelegramChatID

	form.AddInputField("Bot Token", botToken, 50, nil, func(v string) { botToken = v })
	form.AddInputField("Chat ID",   chatID,   20, nil, func(v string) { chatID = v })
	form.AddButton("Save", func() {
		cfg.TelegramBotToken = botToken
		cfg.TelegramChatID   = chatID
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.TelegramBotToken = ""
		cfg.TelegramChatID   = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 12)
}

// ── Generic Webhook ───────────────────────────────────────────────────────────

func showWebhookForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Webhook Configuration")
	webhookURL    := cfg.WebhookURL
	webhookMethod := cfg.WebhookMethod
	if webhookMethod == "" {
		webhookMethod = "POST"
	}

	form.AddInputField("URL",    webhookURL,    50, nil, func(v string) { webhookURL = v })
	form.AddInputField("Method", webhookMethod, 10, nil, func(v string) { webhookMethod = v })
	form.AddButton("Save", func() {
		cfg.WebhookURL    = webhookURL
		cfg.WebhookMethod = webhookMethod
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.WebhookURL    = ""
		cfg.WebhookMethod = "POST"
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 12)
}

// ── Prometheus Alertmanager ───────────────────────────────────────────────────

func showAlertmanagerForm(app *tview.Application, cfg *config.Config, saved *bool, back func()) {
	form := newForm("Prometheus Alertmanager Configuration")
	amURL := cfg.AlertmanagerURL

	form.AddInputField("Alertmanager URL", amURL, 50, nil, func(v string) { amURL = v })
	form.AddButton("Save", func() {
		cfg.AlertmanagerURL = amURL
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Clear", func() {
		cfg.AlertmanagerURL = ""
		saveAndBack(app, cfg, saved, back)
	})
	form.AddButton("Back", func() { back() })

	wrapForm(app, form, 10)
}
