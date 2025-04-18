local rpack = require("rpack.v1")
print("This script templates the userlist.yaml into an AUTHORS.md")
local users = rpack.from_yaml(rpack.read("map:authors.yaml"))
local tmpl_output = rpack.template(rpack.read("rpack:files/authors.md.tmpl"), users)

-- Decide based on config where to write output to
local values = rpack.values()
local output_file = "./AUTHORS.md"
if values.output then
    output_file = values.output_file
end
rpack.write(output_file, tmpl_output)
