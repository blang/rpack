local rpack = require("rpack.v1")
print("Hello, im not implemented yet")

-- Recursive function to print every member of a variable.
-- var: the variable (typically a table) to be traversed.
-- name: a string representing the “name” or "path" of the variable.
-- visited: a table to keep track of already visited tables (for cycle detection).
local function printMembersRecursive(var, name, visited)
    -- Initialize visited table if it wasn't provided.
    visited = visited or {}

    local varType = type(var)
    print(name .. " (" .. varType .. ")")

    -- If var is a table and not already visited, traverse its members.
    if varType == "table" and not visited[var] then
        visited[var] = true -- mark the table as visited

        -- First gather all the keys and sort them for a predictable order.
        local keys = {}
        for k in pairs(var) do
            table.insert(keys, k)
        end
        table.sort(keys, function(a, b)
            return tostring(a) < tostring(b)
        end)

        -- Traverse each key.
        for _, k in ipairs(keys) do
            local value = var[k]
            -- Build a more descriptive name for the nested value.
            local subName = name .. "[" .. tostring(k) .. "]"
            printMembersRecursive(value, subName, visited)
        end
    end
end

-- Example 1: Print the global table _G recursively.
print("----- Global Table _G -----")
printMembersRecursive(_G, "_G")

-- Example 2: Define and print a local variable.
print("\n----- Local Variable rpack -----")
printMembersRecursive(rpack, "rpack")


print("Inputs:")
local inputs = rpack.inputs()
printMembersRecursive(inputs, "inputs")
print("Values:")
local values = rpack.values()
printMembersRecursive(values, "values")

-- Actual action
rpack.copy("rpack:myfile.yaml", "myfile.yaml")
local content = rpack.read_yaml("rpack:myfile.yaml")
table.insert(content.users, "eve")
rpack.write("temp:temporary_output.yaml", rpack.to_yaml(content))
local content2 = rpack.from_yaml(rpack.read("temp:temporary_output.yaml"))
table.insert(content2.users, "oliver")
rpack.write("final_users.yaml", rpack.to_yaml(content2))
