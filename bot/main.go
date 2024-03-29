package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	TriggerKeyword     = "як будзе "
	ErrorMessage       = "Нешта чамусці пайшло ня так. Стварыце калі ласка ішшу на гітхабе https://github.com/slawiko/ru-bel-bot/issues"
	EmptyResultMessage = "Нічога не знайшоў :("
	HelpMessage        = `Спосабы ўзаемадзеяння:

<b>У прываце</b>
Наўпрост пішыце слова на рускай мове.
Увага: тут я лагірую тэкст, які вы напішаце.

<b>У группе (спачатку дадайце мяне ў группу)</b>
Пачніце ваша паведамленне са словаў <code>як будзе</code> і далей слово на русском языке. 
Напрыклад: <code>як будзе письмо</code>
Увага: тут я лагірую толькі факт карыстання гэтай функцыяй.

<b>Убудаваны (inline mode)</b>
Напішыце маё імя, а потым слова. І пачакайце пакуль не ўсплывуць падказкі.
Напрыклад: <code>@jeujik_bot письмо</code>
Увага: тут я лагірую тэкст, які вы напішаце.

Таксама вы можаце не пераходзіць на рускую раскладку і пытацца, напрыклад, слова <code>ўавель</code> ці <code>олівка</code>.

У тым выпадку, калі вы баіцеся дадаць мяне ў вашыя чаты, ці пытаць словы, вы можаце запусціць мяне самастойна. Інструкцыя тут: https://github.com/slawiko/ru-bel-bot/blob/master/README.md#run. Калі нешта незразумела - пішыце ў https://github.com/slawiko/ru-bel-bot/issues

<i>На дадзены момант я ня разумею памылкі ў словах, прабачце.</i>

© Усе пераклады я бяру з https://skarnik.by, аўтар ведае аб гэтым. Дзякуй яму вялікі.`
	StartMessage = `Прывітаннечка. Мяне клічуць Жэўжык, я дапамагаю перайсці на родную мову. Вы можаце пытацца ў мяне слова на рускай, а я адкажу вам на беларускай.

Вы можаце дадаць мяне ў группу і пытацца не выходзячы з дыялогу з сябрамі. За дапамогай клацайце /help`
	DetailedButton = "Падрабязней"
	ShortButton    = "Карацей"
)

const TelegramMessageMaxSize = 4096

var BotApiKey = os.Args[1]
var Version = os.Getenv("VERSION")

func main() {
	bot, err := tgbotapi.NewBotAPI(BotApiKey)
	if err != nil {
		log.Println(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			log.Println("callback") // do not log callback requests, since it could go from group
			handleCallback(bot, update.CallbackQuery)
			continue
		}
		if update.InlineQuery != nil {
			log.Println("inline query", update.InlineQuery.Query)
			handleInlineQuery(bot, &update)
			continue
		}

		if update.Message == nil {
			continue
		}

		if update.Message.ViaBot != nil {
			continue
		}

		if update.Message.IsCommand() {
			log.Println("command", update.Message.Command())
			handleCommand(bot, &update)
		} else if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
			handleGroupMessage(bot, &update)
		} else if update.Message.Chat.IsPrivate() {
			log.Println("private", update.Message.Text)
			handlePrivateMessage(bot, &update)
		}
	}
}

func handleInlineQuery(bot *tgbotapi.BotAPI, update *tgbotapi.Update) {
	if len(update.InlineQuery.Query) <= 3 {
		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID: update.InlineQuery.ID,
		}
		_, err := bot.Request(inlineConf)
		if err != nil {
			log.Println(err)
		}
		return
	}

	normalizedText := PrepareRequestText(update.InlineQuery.Query)

	suggestions, err := getSkarnikSuggestions(normalizedText)
	if err != nil {
		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID: update.InlineQuery.ID,
		}
		_, err = bot.Request(inlineConf)
		if err != nil {
			log.Println(err)
		}
		return
	}

	if len(suggestions) == 0 {
		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID: update.InlineQuery.ID,
		}
		_, err := bot.Request(inlineConf)
		if err != nil {
			log.Println(err)
		}
		return
	}

	articles := []tgbotapi.InlineQueryResultArticle{}

	for i := 0; i < len(suggestions); i++ {
		if i > 2 {
			break
		}

		resp, err := requestSkarnik(suggestions[i])
		if err != nil {
			log.Println(err)
			continue
		}

		sgstTranslation, HTMLSgstTranslation, err := ShortTranslationParse(resp.Body)
		if err != nil {
			log.Println(err)
			continue
		}

		messageText := fmt.Sprintf("<i>%s</i> па беларуску будзе %s", update.InlineQuery.Query, HTMLSgstTranslation)

		article := tgbotapi.NewInlineQueryResultArticleHTML(strconv.Itoa(suggestions[i].ID), suggestions[i].Label, messageText)
		article.Description = sgstTranslation
		articles = append(articles, article)
	}

	results := make([]interface{}, len(articles))
	for i, v := range articles {
		results[i] = v
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: update.InlineQuery.ID,
		Results:       results,
		IsPersonal:    true,
	}
	_, err = bot.Request(inlineConf)
	if err != nil {
		log.Println("Request fail", len(results), err)
		for _, e := range articles {
			log.Println(e)
			if e.InputMessageContent == nil {
				log.Println(e.Description)
			}
		}
	}
}

func sendMsg(bot *tgbotapi.BotAPI, msg tgbotapi.MessageConfig) {
	msg.DisableNotification = true

	_, err := bot.Send(msg)
	if err != nil {
		log.Println(err)
		msg.Text = ErrorMessage
		msg.DisableWebPagePreview = true

		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}
	}
}

func PrepareRequestText(searchTerm string) string {
	cleanSearchTerm := strings.ToLower(strings.TrimSpace(searchTerm))
	cleanSearchTerm = strings.ReplaceAll(cleanSearchTerm, "ў", "щ")
	cleanSearchTerm = strings.ReplaceAll(cleanSearchTerm, "і", "и")
	cleanSearchTerm = strings.ReplaceAll(cleanSearchTerm, "’", "ъ")
	cleanSearchTerm = strings.ReplaceAll(cleanSearchTerm, "'", "ъ")

	return cleanSearchTerm
}

func handleGroupMessage(bot *tgbotapi.BotAPI, update *tgbotapi.Update) {
	requestText := PrepareRequestText(update.Message.Text)

	if strings.HasPrefix(requestText, TriggerKeyword) {
		log.Println("group") // do not log group message requests, since there could be sensitive data
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
		msg.ReplyToMessageID = update.Message.MessageID
		msg.ParseMode = tgbotapi.ModeHTML

		requestText = strings.TrimPrefix(requestText, TriggerKeyword)
		translation, err := Translate(requestText, false)
		if err != nil {
			msg.Text = EmptyResultMessage
			log.Println(err)
		} else {
			if joke() {
				msg.Text = fmt.Sprintf("%s\n%s", jokeMessage(), translation)
			} else {
				msg.Text = translation
			}
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(DetailedButton, marshallCallbackData(requestText, true))),
			)
			msg.ReplyMarkup = keyboard
		}

		if requestText == "подарок" {
			msg.Text += "\n\n<i>Звычайна падарункі хаваюць, каб адрэсат не ўбачыў. Але, як кажуць, калі хочаш нешта схаваць - пакладзі гэта ў самым відавочным месцы. \nЗ навагоднімі падарункамі звязаны пэўныя традыцыі. У некаторых краінах падарункі кладуць у шкарпэткі. У некаторых пад ялінку. У некаторых, нават, пад партфель</i>"
		}

		sendMsg(bot, msg)
	}
}

func handlePrivateMessage(bot *tgbotapi.BotAPI, update *tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
	msg.ReplyToMessageID = update.Message.MessageID
	msg.ParseMode = tgbotapi.ModeHTML

	requestText := PrepareRequestText(update.Message.Text)
	translation, err := Translate(requestText, false)
	if err != nil {
		msg.Text = EmptyResultMessage
		log.Println(err)
	} else {
		msg.Text = translation
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(DetailedButton, marshallCallbackData(requestText, true))),
		)
		msg.ReplyMarkup = keyboard
	}

	sendMsg(bot, msg)
}

func marshallCallbackData(word string, shouldNextBeDetailed bool) string {
	return fmt.Sprintf("%s$%v", word, shouldNextBeDetailed)
}

func unmarshallCallbackData(data string) (string, bool) {
	parts := strings.Split(data, "$")
	isDetailed, _ := strconv.ParseBool(parts[1])
	return parts[0], isDetailed
}

func handleCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	editMsg := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "")
	editMsg.ParseMode = tgbotapi.ModeHTML
	word, isDetailed := unmarshallCallbackData(callback.Data)

	translation, err := Translate(word, isDetailed)
	var buttonText string
	if isDetailed {
		buttonText = ShortButton
	} else {
		buttonText = DetailedButton
	}
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(buttonText, marshallCallbackData(word, !isDetailed))),
	)
	editMsg.ReplyMarkup = &keyboard
	if err != nil {
		log.Println(err)
		editMsg.Text = EmptyResultMessage
	} else {
		editMsg.Text = translation
	}

	_, err = bot.Send(editMsg)
	if err != nil {
		log.Println(err)
	}

	bot.Request(tgbotapi.NewCallback(callback.ID, "")) // for hiding alert. Looks wrong, but donno how else
}

func handleCommand(bot *tgbotapi.BotAPI, update *tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

	switch update.Message.Command() {
	case "start":
		msg.Text = StartMessage
	case "help":
		msg.ParseMode = tgbotapi.ModeHTML
		msg.DisableWebPagePreview = true
		msg.Text = HelpMessage
	case "version":
		if len(Version) > 0 {
			msg.ParseMode = tgbotapi.ModeHTML
			msg.Text = fmt.Sprintf("<a href=\"https://github.com/slawiko/ru-bel-bot/releases/tag/%s\">%s</a>", Version, Version)
		} else {
			msg.Text = "unknown"
		}
	}

	sendMsg(bot, msg)
}
