package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/baneeishaque/adoptium_jdk_go"
	"github.com/codegangsta/cli"
	"github.com/tucnak/store"
	"github.com/ystyle/jvms/utils/file"
	"github.com/ystyle/jvms/utils/jdk"
	"github.com/ystyle/jvms/utils/web"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var version = "2.1.0"

const (
	defaultOriginalpath = "https://raw.githubusercontent.com/ystyle/jvms/new/jdkdlindex.json"
	configFileName      = "jvms.json"
	jdkInstallParentDir = "C:\\Program Files\\Java"
	jdkFileDownloadDir  = "C:\\Program Files\\Java"
)

type Config struct {
	JavaHome          string `json:"java_home"`
	CurrentJDKVersion string `json:"current_jdk_version"`
	Originalpath      string `json:"original_path"`
	Proxy             string `json:"proxy"`
	Store             string `json:"jdk_install_parent_dir"`
	Download          string `json:"jdk_download_dir"`
}

var config = Config{
	JavaHome:          "",
	CurrentJDKVersion: os.Getenv("JAVA_HOME")[strings.LastIndex(os.Getenv("JAVA_HOME"), "\\")+1:],
	Store:             jdkInstallParentDir,
	Download:          jdkFileDownloadDir,
	Originalpath:      defaultOriginalpath,
	Proxy:             "",
}

type JdkVersion struct {
	Version string `json:"version"`
	Url     string `json:"url"`
}

func main() {
	app := cli.NewApp()
	app.Name = "jvms"
	app.Usage = `JDK Version Manager (JVMS) for Windows`
	app.Version = version

	app.CommandNotFound = func(c *cli.Context, command string) {
		log.Fatal("Command Not Found")
	}
	app.Commands = commands()
	app.Before = startup
	app.After = shutdown
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
		os.Exit(1)
	}
}

func commands() []cli.Command {
	return []cli.Command{
		{
			Name:        "init",
			Usage:       "Initialize config file",
			Description: `before init you should clear JAVA_HOME, PATH Environment variable。`,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "java_home",
					Usage: "the JAVA_HOME location",
					Value: filepath.Join(os.Getenv("ProgramFiles"), "jdk"),
				},
				cli.StringFlag{
					Name:  "originalpath",
					Usage: "the jdk Download index file url.",
					Value: defaultOriginalpath,
				},
			},
			Action: func(c *cli.Context) error {
				if c.IsSet("java_home") || config.JavaHome == "" {
					config.JavaHome = c.String("java_home")
				}
				cmd := exec.Command("cmd", "/C", "setx", "JAVA_HOME", config.JavaHome, "/M")
				err := cmd.Run()
				if err != nil {
					return errors.New("set Environment variable `JAVA_HOME` failure: Please run as admin user")
				}
				fmt.Println("set `JAVA_HOME` Environment variable to ", config.JavaHome)

				if c.IsSet("originalpath") || config.Originalpath == "" {
					config.Originalpath = c.String("originalpath")
				}
				path := fmt.Sprintf(`%s/bin;%s;%s`, config.JavaHome, os.Getenv("PATH"), file.GetCurrentPath())
				cmd = exec.Command("cmd", "/C", "setx", "path", path, "/m")
				err = cmd.Run()
				if err != nil {
					return errors.New("set Environment variable `PATH` failure: Please run as admin user")
				}
				fmt.Println("add jvms.exe to `path` Environment variable")
				return nil
			},
		},
		{
			Name:      "list",
			ShortName: "ls",
			Usage:     "List current JDK installations.",
			Action: func(c *cli.Context) error {
				fmt.Println("Installed jdk (* marks in use):")
				v := jdk.GetInstalled(config.Store)
				for i, version := range v {
					str := ""
					if config.CurrentJDKVersion == version {
						str = fmt.Sprintf("%s  * %d) %s", str, i+1, version)
					} else {
						str = fmt.Sprintf("%s    %d) %s", str, i+1, version)
					}
					fmt.Printf(str + "\n")
				}
				if len(v) == 0 {
					fmt.Println("No installations recognized.")
				}
				return nil
			},
		},
		{
			Name:      "install",
			ShortName: "i",
			Usage:     "Install available remote jdk",
			Action: func(c *cli.Context) error {
				if config.Proxy != "" {
					web.SetProxy(config.Proxy)
				}
				v := c.Args().Get(0)
				if v == "" {
					return errors.New("invalid version., Type \"jvms rls\" to see what is available for install")
				}

				if jdk.IsVersionInstalled(config.Store, v) {
					fmt.Println("Version " + v + " is already installed.")
					return nil
				}
				versions, err := getJdkVersions()
				if err != nil {
					return err
				}

				if !file.Exists(config.Download) {
					os.MkdirAll(config.Download, 0777)
				}
				if !file.Exists(config.Store) {
					os.MkdirAll(config.Store, 0777)
				}

				for _, version := range versions {
					if version.Version == v {
						dlzipfile, success := web.GetJDK(config.Download, v, version.Url)
						if success {
							fmt.Printf("Installing JDK %s ...\n", v)

							// Extract jdk to the temp directory
							jdktempfile := filepath.Join(config.Download, fmt.Sprintf("%s_temp", v))
							if file.Exists(jdktempfile) {
								err := os.RemoveAll(jdktempfile)
								if err != nil {
									panic(err)
								}
							}
							err := file.Unzip(dlzipfile, jdktempfile)
							if err != nil {
								return fmt.Errorf("unzip failed: %w", err)
							}

							// Copy the jdk files to the installation directory
							temJavaHome := getJavaHome(jdktempfile)
							err = os.Rename(temJavaHome, filepath.Join(config.Store, v))
							if err != nil {
								return fmt.Errorf("unzip failed: %w", err)
							}

							// Remove the temp directory
							// may consider keep the temp files here
							os.RemoveAll(jdktempfile)

							fmt.Println("Installation complete. If you want to use this version, type\n\njvms switch", v)
						} else {
							fmt.Println("Could not Download JDK " + v + " executable.")
						}
						return nil
					}
				}
				return errors.New("invalid version., Type \"jvms rls\" to see what is available for install")
			},
		},
		{
			Name:      "switch",
			ShortName: "s",
			Usage:     "Switch to use the specified version.",
			Action: func(c *cli.Context) error {
				v := c.Args().Get(0)
				if v == "" {
					return errors.New("you should input a version, Type \"jvms list\" to see what is installed")
				}
				if !jdk.IsVersionInstalled(config.Store, v) {
					fmt.Printf("jdk %s is not installed. ", v)
					return nil
				}
				cmd := exec.Command("cmd", "/C", "setx", "JAVA_HOME", filepath.Join(config.Store, v), "/M")
				err := cmd.Run()
				if err != nil {
					return errors.New("set Environment variable `JAVA_HOME` failure: Please run as admin user")
				}

				config.JavaHome = filepath.Join(config.Store, v)
				fmt.Println("Switch success.\nNow using JDK " + v)
				config.CurrentJDKVersion = v
				return nil
			},
		},
		{
			Name:      "remove",
			ShortName: "rm",
			Usage:     "Remove a specific version.",
			Action: func(c *cli.Context) error {
				v := c.Args().Get(0)
				if v == "" {
					return errors.New("you should input a version, Type \"jvms list\" to see what is installed")
				}
				if jdk.IsVersionInstalled(config.Store, v) {
					fmt.Printf("Remove JDK %s ...\n", v)
					if config.CurrentJDKVersion == v {
						os.Remove(config.JavaHome)
					}
					dir := filepath.Join(config.Store, v)
					e := os.RemoveAll(dir)
					if e != nil {
						fmt.Println("Error removing jdk " + v)
						fmt.Println("Manually remove " + dir + ".")
					} else {
						fmt.Printf(" done")
					}
				} else {
					fmt.Println("jdk " + v + " is not installed. Type \"jvms list\" to see what is installed.")
				}
				return nil
			},
		},
		{
			Name:  "rls",
			Usage: "Show a list of versions available for Download. ",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "a",
					Usage: "list all the version",
				},
			},
			Action: func(c *cli.Context) error {
				if config.Proxy != "" {
					web.SetProxy(config.Proxy)
				}
				versions, err := getJdkVersions()
				if err != nil {
					return err
				}
				for i, version := range versions {
					fmt.Printf("    %d) %s\n", i+1, version.Version)
					if !c.Bool("a") && i >= 9 {
						fmt.Println("\nuse \"jvm rls -a\" show all the versions ")
						break
					}
				}
				if len(versions) == 0 {
					fmt.Println("No availabled jdk veriosn for Download.")
				}

				fmt.Printf("\nFor a complete list, visit %s\n", config.Originalpath)
				return nil
			},
		},
		{
			Name:  "proxy",
			Usage: "Set a proxy to use for downloads.",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "show",
					Usage: "show proxy.",
				},
				cli.StringFlag{
					Name:  "set",
					Usage: "set proxy.",
				},
			},
			Action: func(c *cli.Context) error {
				if c.Bool("show") {
					fmt.Printf("Current proxy: %s\n", config.Proxy)
					return nil
				}
				if c.IsSet("set") {
					config.Proxy = c.String("set")
				}
				return nil
			},
		},
	}
}

func getJavaHome(jdkTempFile string) string {
	var javaHome string
	fs.WalkDir(os.DirFS(jdkTempFile), ".", func(path string, d fs.DirEntry, err error) error {
		if filepath.Base(path) == "javac.exe" {
			temPath := strings.Replace(path, "bin/javac.exe", "", -1)
			javaHome = filepath.Join(jdkTempFile, temPath)
			return fs.SkipDir
		}
		return nil
	})
	return javaHome
}

func getJdkVersions() ([]JdkVersion, error) {
	jsonContent, err := web.GetRemoteTextFile(config.Originalpath)
	if err != nil {
		return nil, err
	}
	var versions []JdkVersion
	err = json.Unmarshal([]byte(jsonContent), &versions)
	if err != nil {
		return nil, err
	}
	//fmt.Println(versions)
	adoptiumJdks := strings.Split(adoptium_jdk_go.ApiListReleases(), "\n")
	for _, adoptiumJdkUrl := range adoptiumJdks {
		fileSeparatorIndex := strings.LastIndex(adoptiumJdkUrl, "/")
		fileName := adoptiumJdkUrl[fileSeparatorIndex+1:]
		fileVersion := strings.TrimSuffix(fileName, ".zip")
		//fmt.Println(fileVersion)
		versions = append(versions, JdkVersion{Version: fileVersion, Url: adoptiumJdkUrl})
	}

	//Azul JDKs
	azulJdks := jdk.AzulJDKs()
	for _, azulJdk := range azulJdks {
		versions = append(versions, JdkVersion{Version: azulJdk.ShortName, Url: azulJdk.DownloadURL})
	}

	//fmt.Println(versions)
	return versions, nil
}

func startup(c *cli.Context) error {
	store.Register(
		"json",
		func(v interface{}) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		},
		json.Unmarshal)

	store.Init("jvms")
	if err := store.Load(configFileName, &config); err != nil {
		return errors.New("failed to load the config:" + err.Error())
	}

	config.JavaHome = os.Getenv("JAVA_HOME")

	if config.Proxy != "" {
		web.SetProxy(config.Proxy)
	}
	return nil
}

func shutdown(c *cli.Context) error {
	if err := store.Save(configFileName, &config); err != nil {
		return errors.New("failed to save the config:" + err.Error())
	}
	return nil
}
