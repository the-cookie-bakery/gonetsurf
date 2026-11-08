package geometry

import "math"

// Affine is a 2D affine transformation.
type Affine struct {
	A float64
	B float64
	C float64
	D float64
	E float64
	F float64
}

// IdentityAffine returns the identity affine transform.
func IdentityAffine() Affine {
	return Affine{A: 1, D: 1}
}

// Multiply combines two affine transforms.
func (matrix Affine) Multiply(other Affine) Affine {
	return Affine{
		A: matrix.A*other.A + matrix.C*other.B,
		B: matrix.B*other.A + matrix.D*other.B,
		C: matrix.A*other.C + matrix.C*other.D,
		D: matrix.B*other.C + matrix.D*other.D,
		E: matrix.A*other.E + matrix.C*other.F + matrix.E,
		F: matrix.B*other.E + matrix.D*other.F + matrix.F,
	}
}

// Inverse returns the inverse affine transform.
func (matrix Affine) Inverse() (Affine, bool) {
	determinant := matrix.A*matrix.D - matrix.B*matrix.C
	if math.Abs(determinant) < 1e-12 {
		return IdentityAffine(), false
	}
	return Affine{
		A: matrix.D / determinant,
		B: -matrix.B / determinant,
		C: -matrix.C / determinant,
		D: matrix.A / determinant,
		E: (matrix.C*matrix.F - matrix.D*matrix.E) / determinant,
		F: (matrix.B*matrix.E - matrix.A*matrix.F) / determinant,
	}, true
}

// Apply transforms a point.
func (matrix Affine) Apply(point Point) Point {
	return Point{
		X: matrix.A*point.X + matrix.C*point.Y + matrix.E,
		Y: matrix.B*point.X + matrix.D*point.Y + matrix.F,
	}
}

// TranslationAffine returns a translation transform.
func TranslationAffine(xPos, yPos float64) Affine {
	return Affine{A: 1, D: 1, E: xPos, F: yPos}
}

// ScaleAffine returns a scale transform.
func ScaleAffine(xScale, yScale float64) Affine {
	return Affine{A: xScale, D: yScale}
}

// RotationAffine returns a rotation transform in radians.
func RotationAffine(radians float64) Affine {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return Affine{A: cosine, B: sine, C: -sine, D: cosine}
}

// SkewAffine returns a skew transform in radians.
func SkewAffine(xAngle, yAngle float64) Affine {
	return Affine{A: 1, C: math.Tan(xAngle), B: math.Tan(yAngle), D: 1}
}

// Matrix4 is a row-major 4x4 homogeneous transform applied to column vectors.
type Matrix4 [16]float64

// IdentityMatrix4 returns the identity 4x4 transform.
func IdentityMatrix4() Matrix4 {
	return Matrix4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// Multiply combines two 4x4 transforms.
func (matrix Matrix4) Multiply(other Matrix4) Matrix4 {
	var result Matrix4
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

// Apply transforms a homogeneous point.
func (matrix Matrix4) Apply(xPos, yPos, zPos float64) (float64, float64, float64, float64) {
	xOut := matrix[0]*xPos + matrix[1]*yPos + matrix[2]*zPos + matrix[3]
	yOut := matrix[4]*xPos + matrix[5]*yPos + matrix[6]*zPos + matrix[7]
	zOut := matrix[8]*xPos + matrix[9]*yPos + matrix[10]*zPos + matrix[11]
	wOut := matrix[12]*xPos + matrix[13]*yPos + matrix[14]*zPos + matrix[15]
	return xOut, yOut, zOut, wOut
}

// Project applies the transform and divides by w.
func (matrix Matrix4) Project(xPos, yPos float64) (float64, float64, bool) {
	xOut, yOut, _, wOut := matrix.Apply(xPos, yPos, 0)
	if math.Abs(wOut) < 1e-12 {
		return 0, 0, false
	}
	return xOut / wOut, yOut / wOut, true
}

// Inverse returns the inverse transform.
func (matrix Matrix4) Inverse() (Matrix4, bool) {
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
			return IdentityMatrix4(), false
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
	var result Matrix4
	for row := 0; row < 4; row++ {
		copy(result[row*4:row*4+4], augmented[row][4:8])
	}
	return result, true
}

// TranslationMatrix4 returns a translation transform.
func TranslationMatrix4(xPos, yPos, zPos float64) Matrix4 {
	return Matrix4{1, 0, 0, xPos, 0, 1, 0, yPos, 0, 0, 1, zPos, 0, 0, 0, 1}
}

// ScaleMatrix4 returns a scale transform.
func ScaleMatrix4(xScale, yScale, zScale float64) Matrix4 {
	return Matrix4{xScale, 0, 0, 0, 0, yScale, 0, 0, 0, 0, zScale, 0, 0, 0, 0, 1}
}

// RotationZMatrix4 returns a rotation about the Z axis.
func RotationZMatrix4(radians float64) Matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return Matrix4{cosine, -sine, 0, 0, sine, cosine, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// RotationXMatrix4 returns a rotation about the X axis.
func RotationXMatrix4(radians float64) Matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return Matrix4{1, 0, 0, 0, 0, cosine, -sine, 0, 0, sine, cosine, 0, 0, 0, 0, 1}
}

// RotationYMatrix4 returns a rotation about the Y axis.
func RotationYMatrix4(radians float64) Matrix4 {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return Matrix4{cosine, 0, sine, 0, 0, 1, 0, 0, -sine, 0, cosine, 0, 0, 0, 0, 1}
}

// PerspectiveMatrix4 returns a perspective projection.
func PerspectiveMatrix4(distance float64) Matrix4 {
	if distance == 0 {
		return IdentityMatrix4()
	}
	return Matrix4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, -1 / distance, 1}
}

// IsAffine2D reports whether the transform is a 2D affine transform.
func (matrix Matrix4) IsAffine2D() bool {
	return math.Abs(matrix[2]) < 1e-12 && math.Abs(matrix[3]) < 1e-12 &&
		math.Abs(matrix[6]) < 1e-12 && math.Abs(matrix[7]) < 1e-12 &&
		math.Abs(matrix[8]) < 1e-12 && math.Abs(matrix[9]) < 1e-12 &&
		math.Abs(matrix[10]-1) < 1e-12 && math.Abs(matrix[11]) < 1e-12 &&
		math.Abs(matrix[12]) < 1e-12 && math.Abs(matrix[13]) < 1e-12 &&
		math.Abs(matrix[14]) < 1e-12 && math.Abs(matrix[15]-1) < 1e-12
}

// Affine2D extracts the 2D affine transform.
func (matrix Matrix4) Affine2D() Affine {
	return Affine{A: matrix[0], B: matrix[4], C: matrix[1], D: matrix[5], E: matrix[3], F: matrix[7]}
}
