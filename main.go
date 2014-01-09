package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

var reportTemplate = `
    <html>
        <title>Test report</title>
        <head>
          <title>Test report</title>
          <STYLE type="text/css">
            .green {
                color: green;
            }
            .red {
                color: red;
            }
          </STYLE>
          <meta name="viewport" content="width=device-width, initial-scale=1.0">
          <link rel="stylesheet" href="http://netdna.bootstrapcdn.com/bootstrap/3.0.3/css/bootstrap.min.css">
          <link rel="stylesheet" href="http://netdna.bootstrapcdn.com/bootstrap/3.0.3/css/bootstrap-theme.min.css">
          <script src="http://netdna.bootstrapcdn.com/bootstrap/3.0.3/js/bootstrap.min.js"></script>
        </head>
    <body>
        {{range $name, $job := .}}
        <h2>{{$name}}</h2>
        <table class="table">
            <tr><td>Test</td><td>Results</td></tr>
            {{range $index, $element := $job}}
            <tr><td>{{$index}}</td><td>{{range $b := $element}}{{if $b}}<span class="glyphicon glyphicon-star green"></span>{{else}}<span class="glyphicon glyphicon-fire red"></span>{{end}} {{end}}</td></tr>
            {{end}}
        </table>

        {{end}}
    </body>
    </html>
`
var usage = `
flaky-detector [base url] [output html file] [list of jobs to check]
example : ./flaky-detector https://jenkins.anyware/platform report.html avop-trunk-communication-integration avop-trunk-cornichon-integration avop-trunk-services-integration
`

func main() {
	fmt.Println("Flaky test detector")

	if len(os.Args) < 4 {
		fmt.Println(usage)
		os.Exit(1)
	}

	//...fuck crapy certificate
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}

	toReport := make(map[string]map[string][]bool)

	for i := 0; i < len(os.Args)-3; i++ {
		jobName := os.Args[3+i]
		fmt.Println("\n\nJOB: " + jobName)
		jobUrl := os.Args[1] + "/job/" + jobName + "/api/json"
		fmt.Println(jobUrl)
		resp, err := client.Get(jobUrl)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			log.Fatal(err)
		}

		var job struct {
			Builds []struct {
				Number int
				Url    string
			}
			Description string
			DisplayName string
			Name        string
		}

		err = json.Unmarshal(body, &job)

		if err != nil {
			log.Fatal(err)
		}

		testHistory := make(map[string][]bool)
		toReport[jobName] = make(map[string][]bool)

		for _, e := range job.Builds {
			url := os.Args[1] + "/job/" + jobName + "/" + strconv.Itoa(e.Number)
			resp, err = client.Get(url + "/api/json")
			if err != nil {
				log.Fatal(err)
			}

			var jobStatus struct {
				Building bool
				Result   string
			}

			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)

			err = json.Unmarshal(body, &jobStatus)

			if err != nil {
				log.Fatal(err)
			}

			if !jobStatus.Building && (jobStatus.Result == "SUCCESS" || jobStatus.Result == "UNSTABLE") {
				fmt.Println("Job Result URL : " + url)
				resp, err = client.Get(url + "/testReport/api/json")
				if err != nil {
					log.Fatal(err)
				}
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					log.Fatal(err)
				}

				var result struct {
					ChildReports []struct {
						Result struct {
							Duration float32
							Suites   []struct {
								Cases []struct {
									ClassName   string
									Duration    float32
									FailedSince int
									Name        string
									Skipped     bool
									Status      string
								}
								Name string
							}
						}
					}
					Suites []struct {
						Cases []struct {
							ClassName   string
							Duration    float32
							FailedSince int
							Name        string
							Skipped     bool
							Status      string
						}
						Name string
					}
				}

				err = json.Unmarshal(body, &result)
				if err != nil {
					log.Fatal(err)
				}

				if len(result.ChildReports) > 0 {
					for _, s := range result.ChildReports[0].Result.Suites {
						for _, t := range s.Cases {
							k := t.ClassName + "#" + t.Name
							if testHistory[k] == nil {
								testHistory[k] = make([]bool, 0, 10)
							}
							testHistory[k] = append(testHistory[k], t.Status != "FAILED" && t.Status != "REGRESSION")
						}
					}
				}
				if len(result.Suites) > 0 {
					for _, s := range result.Suites {
						for _, t := range s.Cases {
							k := t.ClassName + "#" + t.Name
							if testHistory[k] == nil {
								testHistory[k] = make([]bool, 0, 10)
							}
							testHistory[k] = append(testHistory[k], t.Status != "FAILED" && t.Status != "REGRESSION")
						}
					}
				}

			}
		}

		for k, v := range testHistory {

			history := ""
			for _, p := range v {
				if p {
					history = history + "."
				} else {
					history = history + "X"
				}
			}

			// last failed?
			if (len(v) > 0 && !v[0]) || (len(v) > 1 && !v[1]) {
				toReport[jobName][k] = v
			}
		}

	}

	tmpl, err := template.New("test").Parse(reportTemplate)
	if err != nil {
		log.Fatal(err)
	}

	fo, err := os.Create(os.Args[2])

	if err != nil {
		log.Fatal(err)
	}
	defer fo.Close()

	err = tmpl.Execute(fo, toReport)

	if err != nil {
		log.Fatal(err)
	}

}
