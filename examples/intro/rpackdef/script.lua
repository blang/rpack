local rpack = require("rpack.v1")
local values = rpack.values()

-- Copy intro file from rpack to users repo
rpack.copy("rpack:files/intro.md", "./rpack_intro.md")

-- Read the user mapped file from its repo
local users = rpack.from_yaml(rpack.read("map:users.yaml"))
local data = {
    users =  users,
    author = values.author,
}

-- Template the rpacks users.md template with our data
local tmpl_output = rpack.template(rpack.read("rpack:files/users.md.tmpl"), data)

-- Write the template output to the users repo
rpack.write("./rpack_users.md", tmpl_output)
