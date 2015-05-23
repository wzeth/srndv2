//
// config.go
//

package srnd

import (
  "github.com/majestrate/configparser"
  "log"
  "strings"
)

type FeedConfig struct {
  policy FeedPolicy
  addr string
  proxy_type string
  proxy_addr string
}

type APIConfig struct {
  srndAddr string
  frontendAddr string
}
type SRNdConfig struct { 
  daemon map[string]string
  store map[string]string
  database map[string]string
  feeds []FeedConfig
  frontend map[string]string
}

// check for config files
// generate defaults on demand
func CheckConfig() {
  if ! CheckFile("srnd.ini") {
    log.Println("no srnd.ini, creating...")
    err := GenSRNdConfig()
    if err != nil {
      log.Fatal("cannot generate srnd.ini", err)
    }
  }
  if ! CheckFile("feeds.ini") {
    log.Println("no feeds.ini, creating...")
    err := GenFeedsConfig()
    if err != nil {
      log.Fatal("cannot generate feeds.ini", err)
    }
  }
}


// generate default feeds.ini
func GenFeedsConfig() error {
  conf := configparser.NewConfiguration()
  sect := conf.NewSection("10.0.0.1:119")
  sect.Add("proxy-type", "socks4a")
  sect.Add("proxy-host", "127.0.0.1")
  sect.Add("proxy-port", "9050")

  sect = conf.NewSection("10.0.0.1:119")
  sect.Add("overchan.*", "1")
  sect.Add("ano.paste", "0")
  sect.Add("ctl", "1")
 
  return configparser.Save(conf, "feeds.ini")
}

// generate default srnd.ini
func GenSRNdConfig() error {
  conf := configparser.NewConfiguration()

  // nntp related section
  sect := conf.NewSection("nntp")

  sect.Add("instance_name", "test.srndv2.tld")
  sect.Add("bind", "127.0.0.1:1199")
  sect.Add("sync_on_start", "1")

  // article store section
  sect = conf.NewSection("articles")

  sect.Add("store_dir", "articles")
  sect.Add("incoming_dir", "articles")

  // database backend config
  sect = conf.NewSection("database")

  // change this to mysql to use with mariadb or mysql
  sect.Add("type", "postgres")
  // change this to infinity to use with infinity-next
  sect.Add("schema", "srnd")
  sect.Add("host", "127.0.0.1")
  sect.Add("port", "5432")
  sect.Add("user", "root")
  sect.Add("password", "root")
  
  // baked in static html frontend
  sect = conf.NewSection("frontend")
  sect.Add("enable", "1")
  sect.Add("bind", "127.0.0.1:18000")
  sect.Add("name", "web.srndv2.test")
  sect.Add("webroot", "webroot")
  sect.Add("prefix", "/")
  sect.Add("templates", "contrib/templates/default")
  

  return configparser.Save(conf, "srnd.ini")
}

// read config files
func ReadConf() *SRNdConfig {
  
  // begin read srnd.ini

  fname := "srnd.ini"
  var s *configparser.Section
  conf, err := configparser.Read(fname)
  if err != nil {
    log.Fatal("cannot read config file", fname)
    return nil
  }
  var sconf SRNdConfig;

  s, err = conf.Section("nntp")
  if err != nil {
    log.Println("no section 'nntp' in srnd.ini")
    return nil
  }

  sconf.daemon = s.Options()

  s, err = conf.Section("database")
  if err != nil {
    log.Println("no section 'database' in srnd.ini")
    return nil
  }

  sconf.database = s.Options()
  
  s, err = conf.Section("articles")
  if err != nil {
    log.Println("no section 'articles' in srnd.ini")
    return nil
  }

  sconf.store = s.Options()


  // frontend config
  
  s, err = conf.Section("frontend")

  
  if err != nil {
    log.Println("no frontend section in srnd.ini, disabling frontend")
    sconf.frontend = make(map[string]string)   
    sconf.frontend["enable"] = "0"
  } else {
    log.Println("frontend configured in srnd.ini")
    sconf.frontend = s.Options()
    _ , ok := sconf.frontend["enable"]
    if ! ok {
      // default to "0"
      sconf.frontend["enable"] = "0"
    }
    enable , _ := sconf.frontend["enable"]
    if enable == "1" {
      log.Println("frontend enabled in srnd.ini")
    } else {
      log.Println("frontend not enabled in srnd.ini, disabling frontend")
    }
  }
  
  // begin load feeds.ini

  fname = "feeds.ini"
  conf, err = configparser.Read(fname)

  if err != nil { 
    log.Fatal("cannot read config file", fname)
    return nil
  }
  
  sections, err := conf.Find("feed-*")
  if err != nil {
    log.Fatal("failed to load feeds.ini", err)
  }

  var num_sections int
  num_sections = len(sections)
  
  if num_sections > 0 {
    sconf.feeds = make([]FeedConfig, num_sections)
    idx := 0

    // load feeds
    for _, sect := range sections {
      var fconf FeedConfig
      // check for proxy settings
      val := sect.ValueOf("proxy-type")
      if len(val) > 0 && strings.ToLower(val) != "none" {
        fconf.proxy_type = strings.ToLower(val)
        proxy_host := sect.ValueOf("proxy-host")
        proxy_port := sect.ValueOf("proxy-port")
        fconf.proxy_addr = strings.Trim(proxy_host, " ") + ":" + strings.Trim(proxy_port, " ")
      }

      // load feed polcies
      sect_name :=  sect.Name()[5:]
      fconf.addr = strings.Trim(sect_name, " ")
      feed_sect, err := conf.Section(sect_name)
      if err != nil {
        log.Fatal("no section", sect_name, "in feeds.ini")
      }
      opts := feed_sect.Options()
      fconf.policy.rules = make(map[string]string)
      for k, v := range opts {
        fconf.policy.rules[k] = v
      }
      sconf.feeds[idx] = fconf
      idx += 1
    }
  }

  
  
  return &sconf
}