package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

// STPOST sends credentials and a query to Space Track.
func STPOST(postURL string, query string) []byte {
	fmt.Println(postURL, query)
	resp, err := http.PostForm(postURL, url.Values{
		"identity": {os.Getenv("SPACETRACKUSER")},
		"password": {os.Getenv("SPACETRACKPASS")},
		"query":    {query}})
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	return body
}

// FetchSATCAT downloads the full satellite catalog from Space Track and
// writes it to ./satcat.csv.
func FetchSATCAT() {
	queryURL := os.Getenv("SPACETRACKAPIROOT") + "/query/class/satcat/orderby/LAUNCH asc/format/tle/metadata/false"
	resp := STPOST(os.Getenv("SPACETRACKLOGINURL"), queryURL)

	fmt.Println("Writing to ./satcat.csv.")
	err := ioutil.WriteFile("satcat.csv", resp, 0644)

	if err != nil {
		panic(err)
	}
}

// FetchTLEs queries Space Track for all available two-line element sets for a
// satellite with the given noradId.
func FetchTLEs(noradId string, destdir string) {
	// https://www.space-track.org/basicspacedata/query/class/tle/orderby/EPOCH asc/format/tle/metadata/false
	queryURL := os.Getenv("SPACETRACKAPIROOT") +
		"/query/class/tle/NORAD_CAT_ID/" +
		noradId +
		"/orderby/EPOCH asc/format/tle/metadata/false"

	resp := STPOST(os.Getenv("SPACETRACKLOGINURL"), queryURL)
	filename := noradId + ".tle"
	fmt.Printf("Writing to %d/%d.\n", destdir, filename)
	err := ioutil.WriteFile(destdir+"/"+filename, resp, 0644)

	if err != nil {
		panic(err)
	}
}

// ParseSATCATCSV reads a SATCAT in CSV format and returns a slice of SatcatRows.
func ParseSATCATCSV(filename string) []SatcatRow {
	file, err := os.Open(filename)

	if err != nil {
		log.Fatal(err)
	}

	csvReader := csv.NewReader(file)
	var satcatRows []SatcatRow

	// Skip the header
	_, err = csvReader.Read()
	if err == io.EOF {
		log.Fatal(err)
	}

	for {
		// r is a row, abbreviated for the struct literal below
		r, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		satcatRow := SatcatRow{r[0], r[1], r[2], r[3], r[4], r[5],
			r[6], r[7], r[8], r[9], r[10], r[11],
			r[12], r[13], r[14], r[15], r[16], r[17],
			r[18], r[19], r[20], r[21], r[22], r[23],
		}
		satcatRows = append(satcatRows, satcatRow)
	}

	return satcatRows
}

// SatcatRow respresents a row of the Space Track satellite catalog.
type SatcatRow struct {
	IntlDes     string `json:"intldes"`
	NORADID     string `json:"noradid"`
	ObjectType  string `json:"objectType"`
	SatName     string `json:"satName"`
	Country     string `json:"country"`
	LaunchDate  string `json:"launchDate"`
	LaunchSite  string `json:"launchSite"`
	DecayDate   string `json:"decayDate"`
	Period      string `json:"period"`
	Inclination string `json:"inclination"`
	Apogeee     string `json:"apogee"`
	Perigee     string `json:"perigee"`
	Comment     string `json:"comment"`
	CommentCode string `json:"commentCode"`
	RCSValue    string `json:"rcsValue"`
	RCSSize     string `json:"rcsSize"`
	FileID      string `json:"fileID"`
	LaunchYear  string `json:"launchYear"`
	LaunchNum   string `json:"launchNum"`
	LaunchPiece string `json:"launchPiece"`
	IsCurrent   string `json:"isCurrent"`
	ObjectName  string `json:"objectName"`
	ObjectID    string `json:"objectID"`
	ObjectNum   string `json:"objectNum"`
}

// FetchAllTLEs fetches the TLEs for the satellites in the gven satcatRows.
// The TLEs will be placed in .tle files, one for each satellite. If a file
// for a NORAD ID exists in destDir, that satellite will be skipped.
func FetchTLEsForSATCAT(satcatRows []SatcatRow, startRow int, numToFetch int, destDir string) {
	var noradIDQuery string
	var noradIDs []string
	files := make(map[int]*os.File)

	// Iterate over IDs, fetching batches of TLEs
	for _, v := range satcatRows[startRow : startRow+numToFetch] {
		fmt.Printf("%s\n", v.NORADID)
		filename := destDir + "/" + v.NORADID + ".tle"
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		defer f.Close()

		if err != nil {
			if os.IsExist(err) {
				log.Printf("\x1b[31;1m%v. Skipping that NORAD ID.\x1b[0m", err)
			} else {
				log.Fatal(err, "poop")
			}
		} else {
			// Add to the list of NORAD IDs we'll fetch
			noradIDQuery += v.NORADID + ","
			noradIDs = append(noradIDs, v.NORADID)

			noradIDnumerical, err := strconv.Atoi(v.NORADID)
			if err != nil {
				log.Fatal(err)
			}
			files[noradIDnumerical] = f
		}
	}

	if noradIDQuery == "" {
		return
	}

	noradIDQuery = noradIDQuery[:len(noradIDQuery)-1]
	queryURL := os.Getenv("SPACETRACKAPIROOT") +
		"/query/class/tle/NORAD_CAT_ID/" +
		noradIDQuery +
		"/orderby/EPOCH asc/format/tle/metadata/false"

	fmt.Printf("Requesting %s.\n", queryURL)
	t0 := time.Now()
	resp := STPOST(os.Getenv("SPACETRACKLOGINURL"), queryURL)
	t1 := time.Now()
	log.Printf("Received in %v.\n", t1.Sub(t0))

	lines := strings.Split(string(resp), "\n")

	for i := 0; i < len(lines)-1; i++ {
		noradID, err := strconv.Atoi(strings.Trim(lines[i][2:7], " "))
		if err != nil {
			log.Fatal(err)
		}

		if f, ok := files[noradID]; ok {
			if _, err = f.WriteString(lines[i]); err != nil {
				panic(err)
			}
		}
	}
}

// TLELine1 represents the first line of a standard two-line element set
type TLE struct {
	NORADID         uint64  `json:"noradid"`
	Classification  string  `json:"classification"`
	IntlDesignator  string  `json:"intlDesignator"`
	Epoch           float64 `json:"epoch"`
	MnMot1stDeriv   float64 `json:"meanMotion1stDeriv"` // divided by 2
	MnMot2ndDeriv   float64 `json:"meanMotion2ndDeriv"` // divided by 6
	BSTAR           float64 `json:"bstar"`
	Zero            int     `json:"zero"`
	TLENumber       int     `json:"tleNumber"`
	Checksum1       int     `json:"checksum"` // modulo 10
	SatelliteNumber int     `json:"satNumber"`
	Inclination     float32 `json:"inclination"`
	RAAN            float32 `json:"raan"` // right ascension of asc node
	Eccentricity    float32 `json:"eccentricity"`
	ArgOfPerigee    float32 `json:"argumentOfPerigee"`
	MeanAnomaly     float32 `json:"meanAnomaly"`
	MeanMotion      float64 `json:"meanMotion"`
	RevNumber       uint32  `json:"revolutionNumber"`
	Checksum2       int     `json:"checksum"`
}

func (tle TLE) String() string {
	return fmt.Sprintf("NORADID: %f\n", tle.NORADID)
}

// ClockyWocky sends out ticks on the channel c every tickEvery.
func ClockyWocky(tickEvery time.Duration, c chan int64) {
	for {
		time.Sleep(tickEvery)
		c <- 1
	}
}

func main() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	triggerTLEFetch := make(chan int64)
	lastFetched := 0
	satcatRows := make([]SatcatRow, 0)

	versionFlag := flag.Bool("v", false, "Print version number.")
	fetchTLEs := flag.Bool("tle", false, "Fetch Space Track TLEs for satellites listed in the specified satcat.")
	tleDir := flag.String("tle-dir", "./tle", "Directory where TLEs are stored, one file per NORAD ID.")
	batchSize := flag.Int("batch-size", 5, "Max number of NORAD IDs to fetch per TLE request.")
	satcatFilename := flag.String("satcat", "", "Fetch Space Track satellite catalog\n"+
		"If a filename is given for a CSV-formatted SATCAT, use that SATCAT for other operations.")

	flag.Parse()

	if *satcatFilename == "" {
		log.Fatal("Dude, where's my SATCAT at?")
	} else {
		satcatRows = ParseSATCATCSV(*satcatFilename)

		fmt.Printf("Found %d catalog entries.\nFirst NORAD ID: %s\nLast NORAD ID: %s\n",
			len(satcatRows), satcatRows[0].NORADID,
			satcatRows[len(satcatRows)-1].NORADID)

	}

	if *fetchTLEs {
		fmt.Println("Gonna fetch some TLEs for you.")
		FetchTLEsForSATCAT(satcatRows, lastFetched, *batchSize, *tleDir)
		lastFetched += *batchSize
		go ClockyWocky(500000*time.Millisecond, triggerTLEFetch)
	}

	if *versionFlag {
		fmt.Println("satfetch v0.1")
	}

	for {
		select {
		case <-triggerTLEFetch:
			// Set TLE fetch trigger, spacing requests out so we don't hammer Space Track
			FetchTLEsForSATCAT(satcatRows, lastFetched, *batchSize, *tleDir)
			lastFetched += *batchSize
			// fmt.Println(len(satcatRows), lastFetched)
		case <-quit:
			fmt.Println("quitting")
			return
		}
	}
}
