env "local" {
  src = "ent://ent/schema"
  dev = "docker://postgres/16/dev?search_path=public"

  migration {
    dir = "file://migrations/postgres/up"
  }
}
