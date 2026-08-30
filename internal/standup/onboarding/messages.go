package onboarding

import (
	"fmt"
	"html"

	domain "VoiceStandup.ai/internal/core/domain"
)

// welcomeNoTeamMessage — приветствие для пользователя без команды
func welcomeNoTeamMessage(user *domain.Users) string {
	return fmt.Sprintf(
		"Привет, %s! 👋\n\n"+
			"Ты пока не привязан ни к одной команде. Вот что можно сделать:\n"+
			"• Создать свою команду — добавь бота в групповой чат\n"+
			"• Присоединиться к существующей — попроси коллегу прислать инвайт-ссылку",
		html.EscapeString(user.DisplayName),
	)
}

// askRoleMessage — запрос роли после привязки к команде
func askRoleMessage(user *domain.Users) string {
	return fmt.Sprintf(
		"Добро пожаловать в команду, %s! 🎉\n\n"+
			"Напиши, пожалуйста, свою роль (например: разработчик, дизайнер, продакт-менеджер).",
		html.EscapeString(user.DisplayName),
	)
}

// roleSetSuccessMessage — подтверждение после сохранения роли
func roleSetSuccessMessage(user *domain.Users, team *domain.Teams, role string) string {
	return fmt.Sprintf(
		"Отлично, %s! ✅\n\n"+
			"<b>Команда:</b> «%s»\n"+
			"<b>Твоя роль:</b> %s\n\n"+
			"Теперь ты можешь отправлять стендапы:\n"+
			"• 🎤 Голосовое — бот расшифрует и опубликует\n"+
			"• ✍️ Текстовое — бот опубликует как есть",
		html.EscapeString(user.DisplayName),
		html.EscapeString(team.Name),
		html.EscapeString(role),
	)
}

// botAddedToGroupMessage — приветствие при добавлении бота в группу
func botAddedToGroupMessage(chatID int64) string {
	return fmt.Sprintf(
		"Всем привет! 🤖\n\n"+
			"Я — бот для ежедневных стендапов.\n\n"+
			"<b>ID этого чата:</b> <code>%d</code>\n\n"+
			"Чтобы создать команду, используй этот ID. "+
			"Внутри mini app вы можете создать свою команду! А уже затем поделиться ссылкой с участниками!",
		chatID,
	)
}
