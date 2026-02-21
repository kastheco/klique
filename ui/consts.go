package ui

import "github.com/charmbracelet/lipgloss"

var fallbackBannerRaw = `██╗  ██╗██╗     ██╗ ██████╗ ██╗   ██╗███████╗
██║ ██╔╝██║     ██║██╔═══██╗██║   ██║██╔════╝
█████╔╝ ██║     ██║██║   ██║██║   ██║█████╗
██╔═██╗ ██║     ██║██║▄▄ ██║██║   ██║██╔══╝
██║  ██╗███████╗██║╚██████╔╝╚██████╔╝███████╗
╚═╝  ╚═╝╚══════╝╚═╝ ╚══▀▀═╝  ╚═════╝ ╚══════╝`

var FallBackText = lipgloss.JoinVertical(lipgloss.Center,
	GradientText(fallbackBannerRaw, "#F25D94", "#7D56F4"))
