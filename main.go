package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Conf struct {
	SpreadsheetID   string `json:"spreadsheet_id"`
	SheetName       string `json:"sheet_name"`
	CredentialsFile string `json:"credentials_file"`
}

type Response struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
}

const hashCell = "C1"

func main() {
	// Google Sheets configuration

	data, err := os.ReadFile("conf.json")
	if err != nil {
		fmt.Println("Error opening file", err)
		return
	}
	var cfg Conf
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		fmt.Println("Error unmarshal data", err)
		return
	}
	spreadsheetID := cfg.SpreadsheetID
	sheetName := cfg.SheetName
	credentialsFile := cfg.CredentialsFile

	// Set up the Chrome Debugging Protocol client
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to the Confluence page and wait for it to load
	var html string
	err = chromedp.Run(ctx,
		chromedp.Navigate("https://confluence.hflabs.ru/plugins/servlet/mobile?contentId=1181220999#content/view/1181220999"),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		fmt.Println("Error retrieving Confluence page:", err)
		return
	}

	// Calculate the hash of the table data
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		fmt.Println("Error parsing Confluence page:", err)
		return
	}

	var responses []Response
	doc.Find("table tbody tr").Each(func(i int, s *goquery.Selection) {
		cols := s.Find("td")
		if cols.Length() == 2 {
			codeStr := cols.First().Text()
			code, _ := strconv.Atoi(codeStr)
			description := cols.Last().Text()
			responses = append(responses, Response{code, description})
		}
	})

	hash := sha256.New()
	for _, r := range responses {
		hash.Write([]byte(strconv.Itoa(r.Code)))
		hash.Write([]byte(r.Description))
	}
	hashValue := hex.EncodeToString(hash.Sum(nil))

	// Check if the hash value has changed since the last run
	ctx = context.Background()
	sheetsService, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		fmt.Println("Error creating Sheets service:", err)
		return
	}

	readRange := sheetName + "!" + hashCell
	resp, err := sheetsService.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		fmt.Println("Error reading hash value from Google Sheets:", err)
		return
	}

	var lastHashValue string
	if len(resp.Values) > 0 {
		lastHashValue = resp.Values[0][0].(string)
	}

	// If the hash value has not changed, exit without updating Google Sheets
	if hashValue == lastHashValue {
		fmt.Println("Table data has not changed since last run.")
		return
	}

	// Send the table data and new hash value to Google Sheets
	values := make([][]interface{}, len(responses))
	for i, r := range responses {
		values[i] = []interface{}{r.Code, r.Description}
	}

	rangeStr := sheetName + "!A1:B" + strconv.Itoa(len(responses))
	valueRange := &sheets.ValueRange{
		MajorDimension: "ROWS",
		Range:          rangeStr,
		Values:         values,
	}

	_, err = sheetsService.Spreadsheets.Values.Update(spreadsheetID, rangeStr, valueRange).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		fmt.Println("Error updating Google Sheets:", err)
		return
	}

	hashRange := sheetName + "!" + hashCell
	hashValueRange := &sheets.ValueRange{
		MajorDimension: "ROWS",
		Range:          hashRange,
		Values:         [][]interface{}{{hashValue}},
	}

	_, err = sheetsService.Spreadsheets.Values.Update(spreadsheetID, hashRange, hashValueRange).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		fmt.Println("Error updating hash value in Google Sheets:", err)
		return
	}

	fmt.Printf("Table data has been transferred to Google Sheets: https://docs.google.com/spreadsheets/d/%s/edit#gid=<your-sheet-id>\n", spreadsheetID)
}
