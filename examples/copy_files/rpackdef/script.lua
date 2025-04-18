local rpack = require("rpack.v1")
local filepath = require("filepath")
local values = rpack.values()

if values.copy_file1 then
    rpack.copy("rpack:files/file1.txt", "./output/file1.txt")
end

if values.copy_file2 then
    rpack.copy("rpack:files/file2.txt", "./output/file2.txt")
end

if values.copy_inputfile1 then
    rpack.copy("map:inputfile1", "./output/inputfile1")
end

if values.copy_inputdir1 and values.copy_inputdir1.enabled then
    local recursive = values.copy_inputdir1.recursive
    -- returns files and directories, but we are only interested in files
    -- since directories are created implicitely on rpack.copy
    local files, _ = rpack.read_dir("map:inputdir1", recursive)
    for _, inputFile in ipairs(files) do
        -- Extract the location ('map:') and rest of path from the input path
        -- We ignore the location since we write to target under inputdir1
        local _, outPath = filepath.location(inputFile)
        -- Copy map:inputdir1/subdir/subfile.txt to inputdir1/subdir/subfile.txt
        rpack.copy(inputFile, outPath)
    end
end
