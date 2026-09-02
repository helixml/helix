import React from 'react'
import { SquareKanban, type LucideProps } from 'lucide-react'

/** A project is a collection of work, not a directory in the file system. */
const ProjectIcon = (props: LucideProps) => <SquareKanban {...props} />

export default ProjectIcon
