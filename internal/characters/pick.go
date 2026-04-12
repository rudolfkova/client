package characters

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PickTerminal запрашивает GET /api/me/characters и номер в stdin.
// Пустая строка без ошибки — gateway вернул character_service_unconfigured (подключайтесь без character_id).
func PickTerminal(accessToken string) (characterID string, err error) {
	list, err := ListMine(accessToken, 50, 0)
	if errors.Is(err, ErrCharacterServiceUnconfigured) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", fmt.Errorf("нет персонажей для этого аккаунта: создайте через character-web или API, затем перезапустите клиент")
	}

	fmt.Println("\nПерсонажи (введите номер):")
	for i, c := range list {
		desc := strings.TrimSpace(c.Description)
		if desc == "" {
			desc = "—"
		} else if len(desc) > 72 {
			desc = desc[:69] + "..."
		}
		fmt.Printf("  %d) %s\n      %s\n      uuid: %s\n", i+1, c.DisplayName, desc, c.ID)
	}
	fmt.Printf("\nНомер персонажа [1-%d]: ", len(list))
	var line string
	if _, e := fmt.Scanln(&line); e != nil {
		return "", e
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(list) {
		return "", fmt.Errorf("неверный номер: ожидалось 1–%d", len(list))
	}
	return list[n-1].ID, nil
}
