// Helper function for deep equality check
export const isEqual = (obj1: any, obj2: any): boolean => {
  if (obj1 === obj2) return true;
  
  if (
    typeof obj1 !== 'object' || 
    typeof obj2 !== 'object' || 
    obj1 === null || 
    obj2 === null
  ) {
    return false;
  }
  
  const keys1 = Object.keys(obj1);
  const keys2 = Object.keys(obj2);
  
  if (keys1.length !== keys2.length) return false;
  
  for (const key of keys1) {
    if (!keys2.includes(key)) return false;
    
    if (!isEqual(obj1[key], obj2[key])) return false;
  }
  
  return true;
}

export const removeEmptyValues = (obj: any): any => {
  if (Array.isArray(obj)) {
    const filtered = obj.map(removeEmptyValues).filter(v => v !== undefined && v !== null)
    return filtered.length ? filtered : undefined
  } else if (typeof obj === 'object' && obj !== null) {
    const filtered = Object.fromEntries(
      Object.entries(obj)
        .map(([k, v]) => [k, removeEmptyValues(v)])
        .filter(([_, v]) => v !== undefined && v !== null && v !== '')
    )
    return Object.keys(filtered).length ? filtered : undefined
  }
  return obj === '' ? undefined : obj
}