// Optional schema file
#Schema: {
    values: #Values
    inputs: #Inputs
}

#Values: {
    copy_file1?: bool
    copy_file2?: bool
    copy_inputfile1?: bool
    copy_inputdir1?: {
        enabled?: bool
        recursive?: bool
    }
}

#Inputs: [string]: string

