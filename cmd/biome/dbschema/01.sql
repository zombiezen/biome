create table "biomes" (
  "id" text
    not null
    primary key
    check ("id" regexp '([0-9a-f]{2})+'),
  "created_at" timestamp
    not null
    default current_timestamp
    check ("created_at" regexp '[0-9]{4}-[0-9]{2}-[0-9]{2} [0-2][0-9]:[0-5][0-9]:[0-5][0-9](\.[0-9]*)?'),
  "root_host_dir" text
    not null
    check ("root_host_dir" <> '')
);

create table "env_vars" (
  "biome_id" text
    not null
    references "biomes"
      on update cascade
      on delete cascade,
  "name" text
    not null
    check ("name" regexp '[^=]+'),
  "value" text
    not null
    default '',

  primary key ("biome_id", "name")
);

create table "path_parts" (
  "biome_id" text
    not null
    references "biomes"
      on update cascade
      on delete cascade,
  "position" text
    not null
    default 'prepend'
    check ("position" in ('prepend', 'append')),
  "index" integer
    not null
    check ("index" >= 0),
  "directory" text
    not null
    check ("directory" regexp '[^:]+'),

  primary key ("biome_id", "position", "index")
);
