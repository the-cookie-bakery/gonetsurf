package rendering

import "math"

// matrix4 is a row-major 4x4 homogeneous transform applied to column vectors.
type matrix4 [16]float64

func identityMatrix4() matrix4 {
	return matrix4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

func (matrix matrix4) multiply(other matrix4) matrix4 {
	var result matrix4
	for row := 0; row < 4; row++ {
		for column := 0; column < 4; column++ {
			sum := 0.0
			for index := 0; index < 4; index++ {
				sum += matrix[row*4+index] * other[index*4+column]
			}
			result[row*4+column] = sum
		}
	}
	return result
}

func (matrix matrix4) apply(xPos, yPos, zPos float64) (float64, float64, float64, float64) {
	xOut := matrix[0]*xPos + matrix[1]*yPos + matrix[2]*zPos + matrix[3]
	yOut := matrix[4]*xPos + matrix[5]*yPos + matrix[6]*zPos + matrix[7]
	zOut := matrix[8]*xPos + matrix[9]*yPos + matrix[10]*zPos + matrix[11]
	wOut := matrix[12]*xPos + matrix[13]*yPos + matrix[14]*zPos + matrix[15]
	return xOut, yOut, zOut, wOut
}

func (matrix matrix4) project(xPos, yPos float64) (float64, float64, bool) {
	xOut, yOut, _, wOut := matrix.apply(xPos, yPos, 0)
	if math.Abs(wOut) < 1e-12 {
		return 0, 0, false
	}
	return xOut / wOut, yOut / wOut, true
}

func (matrix matrix4) inverse() (matrix4, bool) {
	augmented := [4][8]float64{}
	for row := 0; row < 4; row++ {
		for column := 0; column < 4; column++ {
			augmented[row][column] = matrix[row*4+column]
		}
		augmented[row][4+row] = 1
	}
	for pivot := 0; pivot < 4; pivot++ {
		best := pivot
		for row := pivot + 1; row < 4; row++ {
			if math.Abs(augmented[row][pivot]) > math.Abs(augmented[best][pivot]) {
				best = row
			}
		}
		if math.Abs(augmented[best][pivot]) < 1e-12 {
			return identityMatrix4(), false
		}
		augmented[pivot], augmented[best] = augmented[best], augmented[pivot]
		divisor := augmented[pivot][pivot]
		for column := 0; column < 8; column++ {
			augmented[pivot][column] /= divisor
		}
		for row := 0; row < 4; row++ {
			if row == pivot {
				continue
			}
			factor := augmented[row][pivot]
			for column := 0; column < 8; column++ {
				augmented[row][column] -= factor * augmented[pivot][column]
			}
		}
	}
	var result matrix4
	for row := 0; row < 4; row++ {
		copy(result[row*4:row*4+4], augmented[row][4:8])
	}
	return result, true
}

func translationMatrix4(xPos, yPos, zPos float64) matrix4 {
	return matrix4{1, 0, 0, xPos, 0, 1, 0, yPos, 0, 0, 1, zPos, 0, 0, 0, 1}
}

func scaleMatrix4(xScale, yScale, zScale float64) matrix4 {
	return matrix4{xScale, 0, 0, 0, 0, yScale, 0, 0, 0, 0, zScale, 0, 0, 0, 0, 1}
}

func rotationZMatrix4(radians float64) matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return matrix4{cosine, -sine, 0, 0, sine, cosine, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

func rotationXMatrix4(radians float64) matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return matrix4{1, 0, 0, 0, 0, cosine, -sine, 0, 0, sine, cosine, 0, 0, 0, 0, 1}
}

func rotationYMatrix4(radians float64) matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return matrix4{cosine, 0, sine, 0, 0, 1, 0, 0, -sine, 0, cosine, 0, 0, 0, 0, 1}
}

func perspectiveMatrix4(distance float64) matrix4 {
	if distance == 0 {
		return identityMatrix4()
	}
	return matrix4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, -1 / distance, 1}
}

func (matrix matrix4) isAffine2D() bool {
	return math.Abs(matrix[2]) < 1e-12 && math.Abs(matrix[3]) < 1e-12 &&
		math.Abs(matrix[6]) < 1e-12 && math.Abs(matrix[7]) < 1e-12 &&
		math.Abs(matrix[8]) < 1e-12 && math.Abs(matrix[9]) < 1e-12 &&
		math.Abs(matrix[10]-1) < 1e-12 && math.Abs(matrix[11]) < 1e-12 &&
		math.Abs(matrix[12]) < 1e-12 && math.Abs(matrix[13]) < 1e-12 &&
		math.Abs(matrix[14]) < 1e-12 && math.Abs(matrix[15]-1) < 1e-12
}

func (matrix matrix4) affine2D() affine {
	return affine{A: matrix[0], B: matrix[4], C: matrix[1], D: matrix[5], E: matrix[3], F: matrix[7]}
}

func (matrix matrix4) isIdentity() bool {
	for index, value := range matrix {
		expected := 0.0
		if index == 0 || index == 5 || index == 10 || index == 15 {
			expected = 1
		}
		if math.Abs(value-expected) > 1e-9 {
			return false
		}
	}
	return true
}
