echo '.dump' | sqlite3 config_ca6601.db > config_ca6601.dump
mcedit config_ca6601.dump
cat config_ca6601.dump | sqlite3 config_ca6601.new.db
mv config_ca6601.db config_ca6601.old.db
mv config_ca6601.new.db config_ca6601.db